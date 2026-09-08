package server

import (
	"os"
	"strings"
	"testing"
)

// /pending 376 — one endpoint per ROUND, not one per leg.
//
// # What was there
//
// `deliverToParty` called `setupSharedEndpoint` itself, so a round opened a UDP socket, a QUIC
// transport and a whole DHT server **per party** — each with its own `rate.NewLimiter(250, 64)`,
// which is created per server in `rendezvous.Open`. The machine's aggregate DHT send rate therefore
// scaled with the roster and nothing bounded the total. That is what made "make the round
// concurrent" look like a widening that needed a security review: W legs meant W×250/s leaving one
// host.
//
// # Why it was never necessary
//
// `Publish` and `Fetch` take their seed and salt **per call**, so one rendezvous server already
// serves many targets, and one QUIC transport dials many peers. The per-leg endpoint was an
// artifact of where `setupSharedEndpoint` was called, not a property of a leg: `deliveryCeremony`
// builds a `ceremonyID` out of identity alone and the socket is bolted on afterwards.

// TestABorrowedEndpointOutlivesTheLegThatUsedIt is the ownership rule, and it is the one that
// panics the process when it is wrong.
//
// `ceremonyID.close()`'s own doc: "three of the six plausible orderings panic the process: any that
// closes the mux while the DHT server is still reading it". An endpoint with two closers is that
// panic waiting for a schedule — so a leg must not close what it borrowed, and this is what says so.
func TestABorrowedEndpointOutlivesTheLegThatUsedIt(t *testing.T) {
	dir := t.TempDir()
	shared, closeShared, err := openSharedRendezvous("127.0.0.1:0", dir)
	if err != nil {
		t.Skipf("no endpoint available in this environment: %v", err)
	}
	defer closeShared()

	addr := shared.end.LocalAddr()
	if addr == nil {
		t.Fatal("setup: the shared endpoint reports no address, so nothing below is observable")
	}

	// A leg borrows it and finishes — which is what `defer cer.close()` does at the end of every
	// `deliverToParty`.
	cer := &ceremonyID{}
	cer.borrowEndpoint(shared.end, shared.rz)
	if !cer.borrowedEndpoint {
		t.Fatal("setup: the ceremony does not record the endpoint as borrowed")
	}
	cer.close()

	// THE ASSERTION, and it is a WRITE rather than an address.
	//
	// **`LocalAddr()` was the first cut and it is vacuous** — a closed socket still reports the
	// address it was bound to, so comparing it before and after cannot see a teardown. Measured:
	// with the ownership guard removed from `close()`, that version stayed green. The observable
	// has to be USE, so this sends a datagram, which is what the next leg would do.
	if err := shared.end.Punch(addr); err != nil {
		t.Fatalf("after a borrowing leg closed, the round's endpoint will not send: %v. The leg tore "+
			"down what it borrowed, and every leg after it dials a closed socket", err)
	}

	// And a SECOND borrower works, which is the whole point of lending it.
	second := &ceremonyID{}
	second.borrowEndpoint(shared.end, shared.rz)
	second.close()
	if err := shared.end.Punch(addr); err != nil {
		t.Fatalf("the round's endpoint did not survive two borrowing legs: %v", err)
	}
}

// TestAnOwnedEndpointIsStillTornDown — the other arm, and it is what keeps the flag from being a
// blanket "never close".
//
// A `ceremonyID` that opened its own endpoint must still release it, or every arm outside a round
// leaks a socket, a DHT server and a port-mapping lease for the life of the process.
func TestAnOwnedEndpointIsStillTornDown(t *testing.T) {
	dir := t.TempDir()
	cer := &ceremonyID{}
	if err := cer.setupSharedEndpoint("127.0.0.1:0", dir); err != nil {
		t.Skipf("no endpoint available in this environment: %v", err)
	}
	if cer.borrowedEndpoint {
		t.Fatal("a ceremony that opened its OWN endpoint is marked as borrowing it, so close() " +
			"would leave the socket, the DHT server and the port-mapping lease behind for the life " +
			"of the process")
	}
	cer.close()
}

// TestTheDeliveryRoundOpensOneEndpointForTheWholeWalk pins the routing.
//
// **A source scan, and labelled as one.** What is being asserted is that the round opens the
// endpoint and every leg receives it — a structural fact about one function. Counting live sockets
// would need a real multi-party round, which is tier 4's, and a behavioural check inside this
// package cannot tell one endpoint from four without reaching into the runtime. The ownership rule
// itself is driven for real above.
func TestTheDeliveryRoundOpensOneEndpointForTheWholeWalk(t *testing.T) {
	src, err := os.ReadFile("delivery.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func (s *Server) runDeliveryRound(")
	if start < 0 {
		t.Fatal("runDeliveryRound is not in delivery.go — this scan is reading the wrong thing")
	}
	end := strings.Index(body[start:], "\n// deliverToParty")
	if end < 0 {
		t.Fatal("could not find the end of runDeliveryRound — the landmark moved")
	}
	round := body[start : start+end]

	if n := strings.Count(round, "openSharedRendezvous("); n != 1 {
		t.Errorf("runDeliveryRound calls openSharedRendezvous %d times, want exactly 1. The round "+
			"owns ONE endpoint; opening more puts the per-server DHT send limiter back on a per-leg "+
			"footing, which is what /pending 376 was about", n)
	}
	if !strings.Contains(round, "myFP, payload, shared)") {
		t.Error("runDeliveryRound no longer hands its shared endpoint to deliverToParty, so every " +
			"leg opens its own socket and its own DHT server again")
	}
	if strings.Contains(round, "setupSharedEndpoint(") {
		t.Error("runDeliveryRound opens an endpoint through setupSharedEndpoint rather than the " +
			"round's one door")
	}
}
