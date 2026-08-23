package p2p

import (
	"errors"
	"io"
	"net"
	"os"

	"github.com/quic-go/quic-go"
)

// IsTransportLoss reports whether err is a LOST CHANNEL — something a caller may re-race (P05.S10) —
// as opposed to a decided protocol outcome that must end the exchange.
//
// A WHITELIST of transport failures, never a blacklist of outcomes (S10 grill P1): the
// man-in-the-middle signal (errCommitmentBroken) and every decline/consent sentinel are ordinary
// errors, NOT transport errors, so they fall through to false and terminate — re-racing them would
// retry under a MITM or re-ask a person who already answered. An UNKNOWN error also terminates
// (fail-closed), which is the safe direction: the worst case of not re-racing is a user re-arming,
// while wrongly re-racing could retry under attack.
//
// It lives in p2p because the transport errors it recognises are QUIC's and the net stack's — the
// server classifies re-race decisions but does not own what a dropped session looks like on the wire.
func IsTransportLoss(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	// A QUIC connection or stream that died mid-exchange. A DECIDED outcome never arrives as one of
	// these — the exchange returns a sentinel BEFORE any close — so a QUIC error here is a drop.
	var (
		app  *quic.ApplicationError
		str  *quic.StreamError
		tr   *quic.TransportError
		idle *quic.IdleTimeoutError
	)
	return errors.As(err, &app) || errors.As(err, &str) || errors.As(err, &tr) || errors.As(err, &idle)
}
