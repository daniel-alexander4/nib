package server

import (
	"errors"
	"io"
	"net"
	"os"
)

// isTransportLoss reports whether err is a LOST CHANNEL — re-race it (P05.S10) — rather than a
// decided outcome that must end the ceremony.
//
// A WHITELIST, deliberately, not a blacklist (grill P1): the man-in-the-middle signal
// (p2p errCommitmentBroken) and every decline/consent sentinel sit OUTSIDE the transport errors,
// so "re-race unless it was a decline" would retry the connection UNDER A MITM — the one thing the
// verification exchange exists to refuse. So re-race only on a positively-identified transport
// failure, and default to terminating.
//
// The residue is named and tolerated: a bare io.EOF after the peer decided-then-crashed before its
// refusal byte is indistinguishable from a drop (two-generals). It re-races into a FRESH spoken
// check, so the worst case is one extra confirmation, never a signature the user did not authorise.
func isTransportLoss(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne)
}
