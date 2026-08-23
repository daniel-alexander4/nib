package server

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"

	"nib/internal/p2p"
)

// TestIsTransportLossIsAWhitelist — P05.S10 T04 / grill P1. Re-racing must be a WHITELIST of
// transport failures, never a blacklist: the man-in-the-middle and decline signals sit outside the
// transport errors, and re-racing them would retry under a MITM or re-ask a person who already
// answered. This pins the classification of the actual sentinels the exchange returns.
func TestIsTransportLossIsAWhitelist(t *testing.T) {
	// Transport losses — MUST re-race.
	race := []error{
		io.EOF,
		io.ErrUnexpectedEOF,
		os.ErrDeadlineExceeded,
		fmt.Errorf("send commitment: %w", io.ErrUnexpectedEOF), // the wrapped frame errors verify.go produces
		&net.OpError{Op: "read", Err: errors.New("connection reset by peer")},
	}
	for _, e := range race {
		if !isTransportLoss(e) {
			t.Errorf("isTransportLoss(%v) = false, want true — a lost channel must re-race", e)
		}
	}
	// Decided outcomes — MUST terminate, never re-race.
	terminate := []error{
		nil,
		p2p.ErrCoSignDeclined,
		p2p.ErrVerificationDeclined,
		p2p.ErrConsentTimedOut,
		errors.New("peer's contribution does not match the commitment it sent"), // the MITM signal's text
		errors.New("the peer's attestation does not accept you"),
	}
	for _, e := range terminate {
		if isTransportLoss(e) {
			t.Errorf("isTransportLoss(%v) = true, want false — a DECIDED outcome must not re-race "+
				"(re-racing a decline re-asks a person; re-racing the MITM signal retries under attack)", e)
		}
	}
}
