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
	"net"
	"time"

	"nib/internal/sign"
)

const (
	// handshakeTimeout bounds one peer's handshake, so a connection that opens and
	// says nothing cannot hold an armed session for its whole window while looking
	// live — the same denial the accept loop exists to prevent, one step later. It is
	// per-attempt: a peer that fails is dropped and the listener keeps accepting until
	// the arm window closes.
	//
	// Moved here from internal/server's sessionHandshakeTimeout by P02.S05, VALUE
	// UNCHANGED. It belongs to a transport and not to the server, because what has to
	// be bounded differs per transport — TLS-over-TCP bounds a handshake, QUIC bounds
	// a handshake and the first stream — and the server can no longer see either.
	handshakeTimeout = 30 * time.Second

	// transportSkew backdates the ephemeral cert's NotBefore so a peer whose clock
	// runs a few minutes ahead still accepts it — the identity cert backdates an
	// hour for the same reason; this cert lives minutes, so it backdates minutes.
	transportSkew = 5 * time.Minute
	// transportTTL is the ephemeral cert's forward validity: long enough to finish
	// a co-signing session across reasonable clock skew, short enough to bound the
	// replay window of the per-session ephemeral key if it ever leaked.
	transportTTL = 15 * time.Minute
)

// SessionTLS builds the mTLS config for one co-signing session. It presents an
// ephemeral leaf chained to this user's vault identity, and accepts the peer only
// if the peer's identity SPKI equals pinnedSPKI (the fingerprint pinned out-of-band)
// and the peer's leaf is freshly signed by that identity. Default chain verification
// is disabled — the pinned-peer model replaces it.
//
// The same verification runs whether this side dials or listens, so one constructor
// serves both; pass server=true for the listening side. Read who actually connected
// from the completed handshake (conn.ConnectionState().PeerCertificates) — verification
// has passed by then, so the identity cert there is the pinned peer.
func SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI []byte, server bool) (*tls.Config, error) {
	if len(pinnedSPKI) != sha256.Size {
		return nil, errors.New("pinned fingerprint must be a SHA-256 SPKI hash")
	}
	idCert, idKey, err := sign.ParseIdentity(identityCertPEM, identityKeyPEM)
	if err != nil {
		return nil, err
	}
	leaf, err := mintTransportCert(idCert, idKey)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{leaf},
		// TLS 1.3 only: the peer signs the handshake transcript with the leaf key
		// (CertificateVerify), binding the pinned identity to THIS channel. Pinned
		// explicitly so a future transport that re-defaults the config (e.g. gRPC's
		// applyDefaults) can't drop to 1.2's weaker binding.
		MinVersion: tls.VersionTLS13,
		// We replace chain verification with the pinned-peer check below. With this
		// set, crypto/tls still invokes VerifyPeerCertificate (verifiedChains nil) and
		// still records the peer's chain in ConnectionState.PeerCertificates, so the
		// callback relies solely on rawCerts and callers read the verified peer there.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			_, err := verifyPinnedPeer(rawCerts, pinnedSPKI, time.Now())
			return err
		},
	}
	if server {
		// Require a client cert so the callback runs server-side too; RequireAny
		// (not RequireAndVerify) because we skip the default chain check and do ours.
		cfg.ClientAuth = tls.RequireAnyClientCert
	}
	return cfg, nil
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

// Dial opens a co-signing session to a peer's armed listener at addr, presenting
// this user's identity and accepting only the pinned peer at the TLS handshake. The
// returned conn is a verified mTLS channel; close it when the session ends.
func Dial(addr string, identityCertPEM, identityKeyPEM, pinnedSPKI []byte, timeout time.Duration) (*Conn, error) {
	cfg, err := SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI, false)
	if err != nil {
		return nil, err
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	// The handshake is forced here rather than at the first write, because until it
	// completes there is no verified peer and no exporter — and a dial that reaches
	// something other than the pinned peer should fail as a DIAL, not later as a
	// confusing protocol error inside a session.
	ch, err := TLSChannel(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &Conn{Channel: ch, closer: conn.Close}, nil
}

// Listen returns a session listener bound to addr that drops any peer but the
// pinned one at the TLS handshake. addr is configurable — loopback for tests, a
// routable bind for a real session; the armed listener (P2P 6) owns the bind and
// lifecycle. The signing logic lives in Receive.
func Listen(addr string, identityCertPEM, identityKeyPEM, pinnedSPKI []byte) (Listener, error) {
	cfg, err := SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI, true)
	if err != nil {
		return nil, err
	}
	ln, err := tls.Listen("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	return &tlsListener{ln: ln}, nil
}

// tlsListener adapts a TLS listener to Listener. The handshake, its timeout, and what
// happens to a peer that fails it all live here now — they used to live in the server's
// accept loop, where only TCP could ever have satisfied them.
type tlsListener struct{ ln net.Listener }

func (l *tlsListener) Addr() net.Addr { return l.ln.Addr() }
func (l *tlsListener) Close() error   { return l.ln.Close() }

func (l *tlsListener) Accept() (*Conn, error) {
	c, err := l.ln.Accept()
	if err != nil {
		return nil, err
	}
	tc, ok := c.(*tls.Conn)
	if !ok {
		c.Close()
		return nil, errors.New("not a TLS connection")
	}
	// A handshake has to be time-bounded on its own, or a peer that connects and says
	// nothing holds the session for the whole arm window while looking live. The
	// session core's own SetDeadline replaces this once the exchange starts.
	if err := tc.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		tc.Close()
		return nil, err
	}
	ch, err := TLSChannel(tc)
	if err != nil {
		tc.Close()
		return nil, err // not the pinned peer, or not TLS at all — the caller loops
	}
	if err := tc.SetDeadline(time.Time{}); err != nil {
		tc.Close()
		return nil, err
	}
	return &Conn{Channel: ch, closer: tc.Close}, nil
}

// TLSChannel establishes a Channel over a pinned mTLS connection — the TCP transport
// D14 retains beside QUIC.
//
// This is the one place the handshake is forced, and the reason has not changed with
// the re-typing: the peer's verified identity and the channel's exporter both come into
// existence only when the handshake completes, and the verification string binds both.
// Leaving it to the first write would derive that string from a channel that does not
// exist yet.
func TLSChannel(conn *tls.Conn) (Channel, error) {
	if err := conn.Handshake(); err != nil {
		return Channel{}, err
	}
	cs := conn.ConnectionState()
	fp, err := verifiedPeerFingerprint(cs)
	if err != nil {
		return Channel{}, err
	}
	return Channel{Stream: conn, PeerFP: fp, Export: cs.ExportKeyingMaterial}, nil
}

// verifiedPeerFingerprint reads the verified peer's SPKI fingerprint from a
// completed handshake. PeerCertificates is [leaf, identity] and verification has
// already passed (the handshake would have failed otherwise), so the identity cert
// there is the pinned peer.
func verifiedPeerFingerprint(cs tls.ConnectionState) ([]byte, error) {
	if len(cs.PeerCertificates) < 2 {
		return nil, errors.New("peer presented no identity certificate")
	}
	return sign.FingerprintCert(cs.PeerCertificates[1]), nil
}
