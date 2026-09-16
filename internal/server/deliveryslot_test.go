package server

import (
	"context"
	"strings"
	"testing"

	"nib/internal/ceremony"
)

// TestTheDeliveryArmServingAResumedHopKeepsItsSlot — /pending 500, /pending 385's case.
//
// The delivery arm serves BOTH roles: a party whose hop never reached the convener is dialled on it
// for that hop, and `serveOneSession`'s success return then calls `armDeliveryAfterHop` — which asked
// `slotTaken(armDelivery)`, found the arm that was running the hop, refused, and raised *"Nib could
// not listen for your copy"*. The serving arm then returned as spent, so the party held NO delivery
// arm and the convener's round spent `connectDeadline` failing to reach them.
//
// The slot helper is asked directly because the re-arm's socket half cannot run in a unit test; what
// this pins is the decision the refusal turned on: a slot held for THIS ceremony is this party's
// delivery arm already, and a slot held for another ceremony is a real collision.
func TestTheDeliveryArmServingAResumedHopKeepsItsSlot(t *testing.T) {
	// Armed on the server's OWN session, in place: `Server.sess` is a value holding a mutex, so a
	// session built beside it and copied in would be a copied lock (`go vet` copylocks).
	s := &Server{}
	se := &s.sess
	const mine = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const other = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	held := &ceremonyID{inv: ceremony.Invitation{ID: mine}}
	if !se.armDeliveryForCeremony(held, "127.0.0.1:9", func() {}) {
		t.Fatal("setup: the delivery arm was refused on an empty session")
	}
	// STIMULUS: the slot really is taken, so the answers below are about WHO holds it.
	if !se.slotTaken(armDelivery) {
		t.Fatal("setup: the delivery slot reads free while an arm holds it")
	}
	if !se.deliverySlotHeldFor(mine) {
		t.Error("the delivery slot held for this ceremony is not recognised as this ceremony's — " +
			"the re-arm after a resumed hop refuses its own arm and the party loses its copy")
	}
	if se.deliverySlotHeldFor(other) {
		t.Error("a delivery slot held for a DIFFERENT ceremony reads as this one's; the re-arm " +
			"would report listening for a copy it cannot receive")
	}
	// An EMPTY id never matches, asked of a slot whose arm itself carries no invitation id — a bare
	// `&ceremonyID{}` is how delivery arms are built in this package's own tests. Against the arm
	// above it could not fail: "" never equals "aaaa…".
	bare := &Server{}
	if !bare.sess.armDeliveryForCeremony(&ceremonyID{}, "127.0.0.1:9", func() {}) {
		t.Fatal("setup: the bare delivery arm was refused")
	}
	if bare.sess.deliverySlotHeldFor("") {
		t.Error("an empty ceremony id matched a delivery slot whose arm names no ceremony; an " +
			"invitation that failed to carry an id would be told its arm is already standing")
	}

	// And at the door: `armForDelivery` for the ceremony already holding the slot is not a failure.
	inv := ceremony.Invitation{ID: mine, ConvenerFingerprint: strings.Repeat("ab", 32)}
	if err := s.armForDelivery(context.Background(), inv, nil, nil, strings.Repeat("cd", 32)); err != nil {
		t.Errorf("re-arming delivery for the ceremony whose arm is serving the hop failed: %v", err)
	}
	inv.ID = other
	if err := s.armForDelivery(context.Background(), inv, nil, nil, strings.Repeat("cd", 32)); err == nil {
		t.Error("arming delivery for a DIFFERENT ceremony while the slot is held was not refused")
	}
}
