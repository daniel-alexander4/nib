package p2p

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

// ErrPeerNotPinned is the pinned-peer check's refusal of a certificate chain that is not the pinned
// identity's (/pending 669 #3, ADR-156).
//
// A sentinel where there was a bare `errors.New`, so a DIALLER can tell "what answered at that
// address is not the peer I pinned" from every other reason a dial fails. It survives both
// transports: crypto/tls returns the callback's error as it is, and quic-go keeps it under
// `TransportError.Unwrap` for a local crypto error (`TestARefusalIsNamedOnBothSidesOfBothTransports`).
//
// No wire code: it is returned inside the TLS handshake, before any channel exists to carry a frame;
// the peer learns of it as TLS's own bad_certificate alert (`RefusedByPeer`).
var ErrPeerNotPinned = errors.New("peer identity does not match the pinned peer")

// Refusals counts the handshakes ONE listener refused at the pinned-peer check, and why.
//
// # Why a count, and why here
//
// A refused handshake never surfaces from either listener: `tlsListener`'s handshake goroutine
// returns, and quic-go drops the connection inside the library. That silence is right for the ARM —
// a stranger must not be able to spend it or end it — and wrong for the person waiting, who was shown
// nothing while the one peer they expect was being turned away for a clock that is wrong.
//
// **Counted, never appended.** Anybody who can reach the port can present a certificate, so whatever
// this records is written at a stranger's rate. Two integers, one time and one clock reading are the
// whole of it: no log line, no list, no per-source table. The cost of a million refusals is the cost
// of one.
//
// # What each count means, and who can move it
//
//   - NotPinned: a certificate chain that is not the pinned identity's, or is not a chain at all.
//     ANY host that reaches the port moves this. It says "something else connected", and nothing
//     about the pinned peer.
//   - Clock: the chain IS the pinned identity's — the pin and the leaf's signature are checked first
//     (`verifyPinnedPeer`) — and its validity window is outside this machine's clock. A `Behind`
//     reading needs a leaf dated in our future, which only the holder of the pinned key can mint.
//     An expired one can be REPLAYED: a listener presents its leaf to whoever sends a ClientHello, so
//     anyone may collect the pinned peer's leaf and present it once it has lapsed. So an expired
//     reading never displaces a `Behind` one — see note.
//
// A peer that sends no certificate, or never speaks TLS, is refused before the check and is not
// counted: it presented nothing to be wrong about.
//
// # The dial half, on the same count
//
// An arm that also DIALS (the ceremony's racing arm) meets the mirror image of each: what answered
// at an address was not the pinned peer (WrongAnswer), or the pinned peer turned this machine away
// (RefusedUs), or its leaf is outside this clock (Clock, the same reading). `NoteDial` records
// those, so one arm has one count whichever side of the handshake it was on.
type Refusals struct {
	mu          sync.Mutex
	notPinned   int
	clock       int
	wrongAnswer int
	refusedUs   int
	skew        *ClockSkewError
	last        time.Time
}

// RefusalReport is one reading of a Refusals.
type RefusalReport struct {
	// NotPinned and Clock are the listener's two counts — see Refusals for who can move each.
	NotPinned, Clock int
	// WrongAnswer counts dials answered by something that is not the pinned peer. RefusedUs counts
	// dials the PINNED peer answered and then turned away: this side checks the far side's
	// certificate first, so only the pinned identity gets as far as refusing ours.
	WrongAnswer, RefusedUs int
	// Skew is the clock reading to show, nil when Clock is 0.
	Skew *ClockSkewError
	// Last is when the most recent refusal of either kind happened.
	Last time.Time
}

// Any reports whether anything was refused.
func (r RefusalReport) Any() bool {
	return r.NotPinned > 0 || r.Clock > 0 || r.WrongAnswer > 0 || r.RefusedUs > 0
}

// note records one verification outcome. nil is a peer that passed and records nothing.
func (r *Refusals) note(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = time.Now()
	var skew *ClockSkewError
	if !errors.As(err, &skew) {
		r.notPinned++
		return
	}
	r.noteSkewLocked(skew)
}

// noteSkewLocked counts one clock refusal. A reading only the pinned peer can cause is not replaced
// by one a stranger can replay.
func (r *Refusals) noteSkewLocked(skew *ClockSkewError) {
	r.clock++
	if r.skew == nil || !r.skew.Behind || skew.Behind {
		r.skew = skew
	}
}

// NoteDial records what a DIAL from this arm — or the exchange on a connection it dialled — came to,
// when that is one of the three named refusals; anything else (nobody answered, a dropped channel, a
// decided protocol outcome) records nothing. Nil-safe, so an arm with no listener of this package's
// may call it.
func (r *Refusals) NoteDial(err error) {
	if r == nil || err == nil {
		return
	}
	var skew *ClockSkewError
	isSkew := errors.As(err, &skew)
	notPinned := errors.Is(err, ErrPeerNotPinned)
	refusedUs := RefusedByPeer(err)
	if !isSkew && !notPinned && !refusedUs {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = time.Now()
	switch {
	case isSkew:
		r.noteSkewLocked(skew)
	case notPinned:
		r.wrongAnswer++
	default:
		r.refusedUs++
	}
}

// Report reads the counts. A nil Refusals — a listener that is not one of this package's — has
// refused nothing.
func (r *Refusals) Report() RefusalReport {
	if r == nil {
		return RefusalReport{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return RefusalReport{NotPinned: r.notPinned, Clock: r.clock, WrongAnswer: r.wrongAnswer,
		RefusedUs: r.refusedUs, Skew: r.skew, Last: r.last}
}

// listenTLS is the ONE door a LISTENING config comes through (ADR-009, ADR-156): SessionTLS for the
// server role, with every refusal of the pinned-peer check counted on the Refusals it hands back.
//
// Four constructors listen — `Listen`, `QUICListen`, `QUICListenOn`, `QUICListenHandshakeOn` — and
// each used to call `SessionTLS(…, true)` for itself, so a rule about what a listener does with a
// refused peer had four sites and two transports to miss. `TestEveryListenerCountsItsRefusals` holds
// that none of them builds a server config any other way.
func listenTLS(identityCertPEM, identityKeyPEM, pinnedSPKI []byte) (*tls.Config, *Refusals, error) {
	cfg, err := SessionTLS(identityCertPEM, identityKeyPEM, pinnedSPKI, true)
	if err != nil {
		return nil, nil, err
	}
	r := &Refusals{}
	verify := cfg.VerifyPeerCertificate
	cfg.VerifyPeerCertificate = func(rawCerts [][]byte, chains [][]*x509.Certificate) error {
		err := verify(rawCerts, chains)
		r.note(err)
		return err
	}
	return cfg, r, nil
}

// tlsAlertBadCertificate is TLS's bad_certificate alert (RFC 8446 §6): what crypto/tls sends when
// VerifyPeerCertificate refuses, over TCP and — as CRYPTO_ERROR 0x100+42 — over QUIC.
const tlsAlertBadCertificate = 42

// RefusedByPeer reports whether err is the FAR side refusing the certificate THIS side presented —
// the dialler's view of a listener's pinned-peer refusal (/pending 669 #3).
//
// **It cannot say which refusal.** The far side's pin check and its clock check both end in the same
// bad_certificate alert, and an alert carries no text, so "you are not who I pinned" and "your
// certificate's dates are outside my clock" are one signal here. A caller words it as both.
//
// **It arrives at the dial OR at the first read, on both transports, and a caller must look in
// both places.** Under TLS 1.3 the client's handshake completes BEFORE the server reads its
// certificate: over TCP the dial then always succeeds and the refusal is the first read's error, and
// over QUIC it is a race between the two — measured, a dial to a refusing listener returned a
// connection with no error.
func RefusedByPeer(err error) bool {
	var tr *quic.TransportError
	if errors.As(err, &tr) && tr.Remote && tr.ErrorCode == quic.TransportErrorCode(0x100+tlsAlertBadCertificate) {
		return true
	}
	// crypto/tls's alert type is unexported; what it exports is this shape and this text, and the
	// test above reads both off a real handshake so a change in either fails by name.
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "remote error" && op.Err != nil && op.Err.Error() == "tls: bad certificate"
}
