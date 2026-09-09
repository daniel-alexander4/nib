package server

import (
	"os"
	"strings"
	"testing"
)

// A words-don't-match verdict on the INITIATE side is the man-in-the-middle signal, and
// verify.go's own doc requires it "must never be reported as a network error." The initiate
// handler is the one synchronous caller a user waits on, so it is where the sentence gets
// written; the receive side already ends the ceremony on it. This guard asserts the two
// verification sentinels are lifted BEFORE the writeConnectDiagnosis fallthrough — because a
// decline that falls through renders as a 502 "could not connect" (and may show an unrelated
// D19 cause), which is exactly the retry-inviting advice verify.go forbids under an active MITM.
//
// A source-order guard rather than an HTTP drive, following consenttimeout_test.go's shape: the
// error->response mapping is inline in the handler and the p2p-level tests drive p2p.Initiate
// directly, so nothing else covers the HTTP mapping. The STIMULUS assertions below refuse to
// pass if the handler was renamed away or either sentinel/branch vanished.
func TestInitiateLiftsTheMITMSignalBeforeTheNetworkDiagnosis(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	// **`runHopDial`, not `handleSessionInitiate` (P01.S02b).** The dial — and with it every
	// error-mapping arm this guard reads — was extracted so `/api/ceremony/hop` runs the same
	// sequence rather than a second copy. The ordering asserted below is unchanged; only its home is.
	const marker = "func (s *Server) runHopDial("
	start := strings.Index(s, marker)
	if start < 0 {
		t.Fatalf("runHopDial not found in session.go — this guard has gone blind")
	}
	// Bound the search to this function: up to the next top-level func.
	rest := s[start+len(marker):]
	end := strings.Index(rest, "\nfunc ")
	if end < 0 {
		end = len(rest)
	}
	body := rest[:end]

	declined := strings.Index(body, "errors.Is(err, p2p.ErrVerificationDeclined)")
	timedOut := strings.Index(body, "errors.Is(err, p2p.ErrVerificationTimedOut)")
	diag := strings.Index(body, "s.writeConnectDiagnosis(w,")

	// STIMULUS: all three must be present, or the order check below is vacuous.
	if declined < 0 {
		t.Fatal("handleSessionInitiate does not lift p2p.ErrVerificationDeclined — a words-don't-match " +
			"MITM verdict would fall through to the network-error diagnosis (verify.go forbids this)")
	}
	if timedOut < 0 {
		t.Fatal("handleSessionInitiate does not lift p2p.ErrVerificationTimedOut")
	}
	if diag < 0 {
		t.Fatal("handleSessionInitiate no longer calls writeConnectDiagnosis — this guard has gone blind")
	}
	// ORDER: the MITM signal is reported as the security event it is, before the network fallthrough.
	if declined > diag {
		t.Errorf("p2p.ErrVerificationDeclined is lifted AFTER writeConnectDiagnosis (%d > %d) — the MITM "+
			"signal would be reported as a connection failure, inviting a retry under attack", declined, diag)
	}
	if timedOut > diag {
		t.Errorf("p2p.ErrVerificationTimedOut is lifted AFTER writeConnectDiagnosis (%d > %d)", timedOut, diag)
	}
}
