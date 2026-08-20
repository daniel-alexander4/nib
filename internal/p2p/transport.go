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
	"sync/atomic"
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
			_, err := verifyPinnedPeer(rawCerts, pinnedSPKI, timeNow())
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

// timeNow is the clock the pinned-peer check reads, indirected for one reason only:
// **the property that matters about D19's cause 5 is that the typed error survives the TLS
// boundary**, and that cannot be asserted by calling verifyPinnedPeer directly.
//
// crypto/tls could plausibly wrap, replace or discard an error returned from
// VerifyPeerCertificate — it sends an alert and the peer sees something quite different —
// so a unit test of the error type would be the vacuous version of this check: green while
// no caller could ever recover the cause. Measured: it survives, and now a test says so.
// It is an atomic.Value and not a plain var because handshakes run concurrently: a test
// swapping the clock races every in-flight VerifyPeerCertificate, and `go test -race`
// says so. A seam added for testability that introduces a data race into the thing it
// tests is worse than no seam.
var clock atomic.Value // func() time.Time

func init() { clock.Store(time.Now) }

func timeNow() time.Time { return clock.Load().(func() time.Time)() }

// setClock swaps the clock and returns a restore func. Tests only.
func setClock(f func() time.Time) func() {
	prev := clock.Load().(func() time.Time)
	clock.Store(f)
	return func() { clock.Store(prev) }
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
	//
	// The refusal carries the DIRECTION and the MAGNITUDE, which is D19's fifth cause.
	// Both numbers are in hand here and were thrown away: the error said "expired or not
	// yet valid", which is two opposite facts in one sentence and names neither. Cause 5
	// is the only one of the five that names a fix a user can perform in ten seconds, and
	// before D35 it arrived as cause 4's "something else".
	if now.Before(leaf.NotBefore) {
		// Their certificate starts in OUR future: our clock is behind theirs.
		return nil, &ClockSkewError{Behind: true, By: leaf.NotBefore.Sub(now)}
	}
	if now.After(leaf.NotAfter) {
		// It ended in our past. Ambiguous by construction — a certificate genuinely older
		// than transportTTL looks identical to a clock that is far enough ahead — so the
		// message says what was observed and offers the clock as the likely cause rather
		// than asserting it.
		return nil, &ClockSkewError{Behind: false, By: now.Sub(leaf.NotAfter)}
	}
	return fp, nil
}

// ClockSkewError is D19's fifth cause: the peer's ephemeral certificate was refused on its
// validity window, and the window says by how much and in which direction.
//
// A typed error rather than a message, and it lives here rather than in the server, because
// the numbers exist only at the refusal site — `verifyPinnedPeer` is the one place that
// holds `now`, `NotBefore` and `NotAfter` together, and it is called from inside TLS's
// VerifyPeerCertificate hook where nothing else can see them.
//
// **Note the asymmetry, because a message that ignores it will be wrong half the time.**
// `transportSkew` backdates NotBefore by 5 minutes and `transportTTL` runs 15 minutes
// forward, so the two directions have different tolerances and both sides verify each
// other. A skew that trips one machine may not trip the other.
type ClockSkewError struct {
	// Behind is true when THIS machine's clock is behind the peer's — their certificate
	// is not yet valid here. False means their certificate has already expired here,
	// which is either a genuinely old certificate or a clock that is ahead.
	Behind bool
	// By is how far outside the window we are, which is a LOWER BOUND on the skew: the
	// window has its own width, so a clock can be wrong by up to transportSkew (behind)
	// or transportTTL (ahead) without tripping anything at all.
	By time.Duration
}

func (e *ClockSkewError) Error() string {
	// Rounded to the minute: the sub-second precision is noise against a human clock, and
	// D19 asks for a sentence somebody can act on.
	by := e.By.Round(time.Minute)
	if by == 0 {
		by = time.Minute // "about 0 minutes" is not a sentence
	}
	if e.Behind {
		return fmt.Sprintf("this machine's clock is at least %s behind the other side's — "+
			"check the date and time on this computer", roughly(by))
	}
	return fmt.Sprintf("the other side's certificate expired %s ago here, which usually "+
		"means this machine's clock is at least that far ahead — check the date and time "+
		"on this computer", roughly(by))
}

// roughly renders a duration the way a person would say it.
func roughly(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d minute(s)", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%.1f hour(s)", d.Hours())
	default:
		return fmt.Sprintf("%.1f day(s)", d.Hours()/24)
	}
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
	// Minted on the SAME clock the verifier reads, so a skewed machine mints a skewed
	// certificate — which is what a genuinely wrong clock does. Reading time.Now() here
	// while the check reads timeNow() skews half the machine, and a test built on that can
	// only ever drive one direction.
	now := timeNow()
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
