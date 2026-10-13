package server

import (
	"net"
	"testing"

	"nib/internal/p2p"
)

// TestADisplacedPolicyArmReleasesItsEndpoint — /pending 615.
//
// The displacement (`yieldPolicyArmLocked`, inside `armIn` since `/pending 623`) emptied the slot, cancelled and closed the listener, and never closed the
// ceremony: the displaced goroutine's `defer disarmCeremony(cer)` then matched nothing, so the
// shared UDP socket, the rendezvous server and the port-mapping refresh lived until the process
// exited — one set per displacement.
//
// Driven at the struct, for `TestASecondArmCannotOrphanALiveCeremony`'s reason, but over a REAL
// endpoint: "closed" is asserted as the socket's port being free to bind again, not as a flag.
func TestADisplacedPolicyArmReleasesItsEndpoint(t *testing.T) {
	end, err := p2p.NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP socket to open here: %v", err)
	}
	cer := &ceremonyID{end: end}
	t.Cleanup(cer.close)
	addr := end.LocalAddr().String()

	// STIMULUS: while the arm is live the port is held, or "free afterwards" proves nothing.
	if pc, lerr := net.ListenPacket("udp", addr); lerr == nil {
		pc.Close()
		t.Fatalf("setup: %s could be bound while the endpoint was open", addr)
	}

	var se session
	cancelled := false
	if !se.armCeremonyByPolicy(cer, addr, func() { cancelled = true }) {
		t.Fatal("setup: the policy arm was refused, so there is nothing to displace")
	}
	// A second POLICY arm does not displace the first: which sweep ran last would then decide what
	// the machine is armed for (`rearmCeremoniesPreferring`).
	if se.armCeremonyByPolicy(&ceremonyID{}, "127.0.0.1:4", func() {}) || cancelled {
		t.Fatal("a policy arm displaced another policy arm")
	}
	// An explicit arm is what displaces: the user's own, installed over the policy arm.
	mine := &ceremonyID{}
	if !se.armCeremony(mine, "127.0.0.1:1", func() {}) {
		t.Fatal("setup: the user's explicit arm did not take the slot from the policy arm")
	}
	if !cancelled {
		t.Error("the displaced arm's goroutine was not cancelled")
	}
	pc, lerr := net.ListenPacket("udp", addr)
	if lerr != nil {
		t.Fatalf("the displaced ceremony's UDP endpoint %s is still open after displacement (%v) — "+
			"nothing else will ever close it, because its arm no longer holds the slot", addr, lerr)
	}
	pc.Close()

	// A user's own arm is not displaced — by another explicit arm or by a policy one — so its
	// ceremony is not closed either.
	if se.armCeremony(&ceremonyID{}, "127.0.0.1:2", func() {}) {
		t.Fatal("a user's own arm was displaced by a second explicit arm")
	}
	if se.armCeremonyByPolicy(&ceremonyID{}, "127.0.0.1:3", func() {}) {
		t.Fatal("a user's own arm was displaced by a policy arm")
	}
	mine.mu.Lock()
	closed := mine.closed
	mine.mu.Unlock()
	if closed {
		t.Error("a ceremony whose arm was NOT displaced was closed")
	}
}
