package p2p

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"nib/internal/sign"
)

const (
	// transportSkew backdates the ephemeral cert's NotBefore so a peer whose clock
	// runs a few minutes ahead still accepts it — the identity cert backdates an
	// hour for the same reason; this cert lives minutes, so it backdates minutes.
	transportSkew = 5 * time.Minute
	// transportTTL is the ephemeral cert's forward validity: long enough to finish
	// a co-signing session across reasonable clock skew, short enough to bound the
	// replay window of the per-session ephemeral key if it ever leaked.
	transportTTL = 15 * time.Minute
)

// VerifiedPeer carries the pinned identity confirmed during a session's TLS
// handshake. SessionTLS returns one; its fingerprint is set — and only set — when
// VerifyPeerCertificate accepts the peer, so an attestation's accepted-peer is
// threaded from the cryptographically verified handshake, never a re-derived or
// user-supplied value.
type VerifiedPeer struct {
	mu sync.Mutex
	fp []byte
}

func (vp *VerifiedPeer) set(fp []byte) {
	vp.mu.Lock()
	vp.fp = append([]byte(nil), fp...)
	vp.mu.Unlock()
}

// Fingerprint returns the SHA-256 SPKI of the verified peer identity, or nil if no
// handshake has passed verification yet. Read it only after a successful handshake.
func (vp *VerifiedPeer) Fingerprint() []byte {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	return append([]byte(nil), vp.fp...)
}

// SessionTLS builds the mTLS config for one co-signing session. It presents an
// ephemeral leaf chained to this user's vault identity, and accepts the peer only
// if the peer's identity SPKI equals pinnedSPKI (the fingerprint pinned out-of-band)
// and the peer's leaf is freshly signed by that identity. Default chain verification
// is disabled — the pinned-peer model replaces it — so the returned VerifiedPeer is
// the authority on who connected.
//
// The same verification runs whether this side dials or listens, so one constructor
// serves both; pass server=true for the listening side.
func SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI []byte, server bool) (*tls.Config, *VerifiedPeer, error) {
	if len(pinnedSPKI) != sha256.Size {
		return nil, nil, errors.New("pinned fingerprint must be a SHA-256 SPKI hash")
	}
	idCert, idKey, err := sign.ParseIdentity(identityCertPEM, identityKeyPEM)
	if err != nil {
		return nil, nil, err
	}
	leaf, err := mintTransportCert(idCert, idKey)
	if err != nil {
		return nil, nil, err
	}
	vp := &VerifiedPeer{}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{leaf},
		// TLS 1.3 only: the peer signs the handshake transcript with the leaf key
		// (CertificateVerify), binding the pinned identity to THIS channel. Pinned
		// here so grpc's applyDefaults can't fall back to 1.2's weaker binding.
		MinVersion: tls.VersionTLS13,
		// We replace chain verification with the pinned-peer check below. With this
		// set, crypto/tls still invokes VerifyPeerCertificate (verifiedChains nil),
		// so the callback relies solely on rawCerts.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			fp, err := verifyPinnedPeer(rawCerts, pinnedSPKI, time.Now())
			if err != nil {
				return err
			}
			vp.set(fp)
			return nil
		},
	}
	if server {
		// Require a client cert so the callback runs server-side too; RequireAny
		// (not RequireAndVerify) because we skip the default chain check and do ours.
		cfg.ClientAuth = tls.RequireAnyClientCert
	}
	return cfg, vp, nil
}

// verifyPinnedPeer is the pinned-peer check, run inside VerifyPeerCertificate on
// both roles. rawCerts[0] is the peer's ephemeral leaf, rawCerts[1] its identity
// cert. It returns the verified identity fingerprint only if every check passes —
// a single error-return path, no early success.
func verifyPinnedPeer(rawCerts [][]byte, pinnedSPKI []byte, now time.Time) ([]byte, error) {
	if len(rawCerts) < 2 {
		return nil, errors.New("peer presented an incomplete certificate chain")
	}
	leaf, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return nil, fmt.Errorf("peer leaf certificate: %w", err)
	}
	idCert, err := x509.ParseCertificate(rawCerts[1])
	if err != nil {
		return nil, fmt.Errorf("peer identity certificate: %w", err)
	}
	// PIN: the peer's identity key must be exactly the one pinned out-of-band.
	fp := sign.FingerprintCert(idCert)
	if subtle.ConstantTimeCompare(fp, pinnedSPKI) != 1 {
		return nil, errors.New("peer identity does not match the pinned peer")
	}
	// SIGNATURE: the live leaf must be signed by that pinned identity key. Use the
	// low-level CheckSignature, NOT leaf.CheckSignatureFrom(idCert): the identity is
	// self-signed and not a CA (no IsCA, no KeyUsageCertSign), so CheckSignatureFrom
	// returns a constraint violation. CheckSignature verifies the key signed the
	// bytes with no CA-bit enforcement — the correct check for a pinned-peer system.
	if err := idCert.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature); err != nil {
		return nil, fmt.Errorf("peer leaf not signed by its pinned identity: %w", err)
	}
	// FRESHNESS: bounds the replay window of a leaked per-session ephemeral key.
	if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
		return nil, errors.New("peer transport certificate is expired or not yet valid")
	}
	return fp, nil
}

// mintTransportCert generates a fresh ephemeral ECDSA key and a short-lived leaf
// certificate signed by the vault identity key. The leaf — not the identity key —
// is the live TLS key, so a TLS-layer signing-oracle or fault attack can't reach
// the identity that signs documents; the identity only mints these certs offline.
func mintTransportCert(idCert *x509.Certificate, idKey crypto.Signer) (tls.Certificate, error) {
	ephKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "nib-transport"},
		NotBefore:             now.Add(-transportSkew),
		NotAfter:              now.Add(transportTTL),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, idCert, &ephKey.PublicKey, idKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der, idCert.Raw}, PrivateKey: ephKey}, nil
}
