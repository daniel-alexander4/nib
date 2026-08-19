package p2p

import (
	"errors"
	"io"
	"time"
)

// Stream is the byte-level surface the session core needs: a reliable, ordered,
// length-prefix-framed conversation with a deadline on it.
//
// Deliberately three methods and not `net.Conn`. The core never dials, never asks who
// it is talking to at the socket level, and never closes — its callers own all three —
// so naming net.Conn would advertise a surface the core does not use and a QUIC stream
// does not have in the same shape.
type Stream interface {
	io.Reader
	io.Writer
	// SetDeadline bounds both directions. The core sets it twice per session: once
	// before anything crosses the wire, and again after consent, so a user's
	// deliberation cannot spend the budget the signed document's write needs.
	SetDeadline(t time.Time) error
}

// Exporter derives keying material from the channel itself — RFC 5705's TLS exporter,
// which is `tls.ConnectionState.ExportKeyingMaterial` for both transports Nib uses.
//
// It is a function and not a `tls.ConnectionState` because the core needs 32 bytes
// bound to this channel and nothing else in that struct; passing the state would let a
// later change reach for the cipher suite or the peer chain and quietly re-couple the
// core to TLS. What it must NOT become is optional — see Channel.check.
type Exporter func(label string, context []byte, length int) ([]byte, error)

// Channel is the whole transport surface of a session, and it is the line D14 draws:
// below it is TCP-with-TLS or QUIC, above it is neither.
//
// Its three fields are the three things the core cannot derive for itself, and each is
// a thing a transport has ALREADY established. The core does not handshake, does not
// verify a certificate, and does not decide who the peer is; it is handed the answers
// and refuses to run without them.
type Channel struct {
	// Stream carries the length-prefixed frames.
	Stream Stream

	// PeerFP is the peer's identity SPKI fingerprint, **already verified by the
	// transport**. For both transports that means the pinned-peer callback in
	// SessionTLS ran and matched; the core takes it as settled, which is precisely
	// why it must not be empty.
	PeerFP []byte

	// Export binds the spoken verification to this channel (D4). Without it the
	// verification string would be four words two parties agreed on over *some*
	// channel, which is what a relay in the middle would also be able to arrange.
	Export Exporter
}

// errChannelIncomplete reports a Channel missing a field the core cannot proceed without.
var errChannelIncomplete = errors.New("incomplete channel")

// check refuses a Channel that is not fully established.
//
// Every field here is a security property that has already been established elsewhere,
// which is exactly what makes a zero value dangerous: a Channel{Stream: s} compiles,
// carries no peer identity and no channel binding, and would run a whole session
// against an unauthenticated stranger with a verification string bound to nothing. The
// four entry points are the security boundary, so they fail closed — the same argument
// runVerification already makes for a nil Verifier.
func (c Channel) check() error {
	switch {
	case c.Stream == nil:
		return errors.New("incomplete channel: no stream")
	case len(c.PeerFP) == 0:
		return errors.New("incomplete channel: no verified peer fingerprint")
	case c.Export == nil:
		return errors.New("incomplete channel: no keying-material exporter")
	}
	return nil
}
