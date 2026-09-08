package discovery

import (
	"encoding/binary"
	"testing"
)

// The hop in the announcement (announcement format v3).
//
// # What it is for
//
// ADR-010 added the transport because "a port without its transport is not an address". This is the
// same sentence one level up: a machine can hold a hop arm and a delivery arm at once, pinned to
// the same peer, and they do opposite things — one can co-sign and serve a stored contribution, the
// other confirms the spoken check without a human and can do neither. Announcing a bare port made
// them indistinguishable on the link, so a dialer raced them and took whichever answered first.

func base(hop int) Announcement {
	return Announcement{Name: "one two three four five six", Port: 9000, Transport: TransportQUIC, Hop: hop}
}

// TestTheHopSurvivesTheWire is the round trip, and it drives HopNone alongside real hops because
// the sentinel is the half a naive encoding gets wrong.
func TestTheHopSurvivesTheWire(t *testing.T) {
	for _, hop := range []int{0, 1, 31, 63, maxHop, HopNone} {
		b, err := base(hop).Encode()
		if err != nil {
			t.Fatalf("hop %d does not encode: %v", hop, err)
		}
		got, err := Parse(b)
		if err != nil {
			t.Fatalf("hop %d does not parse back: %v", hop, err)
		}
		if got.Hop != hop {
			t.Errorf("hop %d came back as %d — a dialer would be sent to the wrong arm", hop, got.Hop)
		}
	}
}

// TestHopZeroIsNotTheSameAsNoHop is the whole reason the sentinel is not zero.
//
// Hop 0 is the convener's own index — an ordinary, real hop. If it shared an encoding with "this
// arm has no ceremony", every manual arm would advertise itself as the convener's hop and a
// ceremony dial would match it.
func TestHopZeroIsNotTheSameAsNoHop(t *testing.T) {
	zero, err := base(0).Encode()
	if err != nil {
		t.Fatal(err)
	}
	none, err := base(HopNone).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if string(zero) == string(none) {
		t.Fatal("hop 0 and HopNone encode identically. Hop 0 is the convener's own index, so an " +
			"arm with no ceremony would advertise itself as that hop and a ceremony dial would " +
			"match it")
	}
	z, _ := Parse(zero)
	n, _ := Parse(none)
	if z.Hop != 0 || n.Hop != HopNone {
		t.Fatalf("round trip collapsed the two: zero->%d, none->%d", z.Hop, n.Hop)
	}
}

// TestAVersionTwoAnnouncementIsRefused — the format's own rule, applied to this bump.
//
// `version`'s doc: an announcement whose version is not this one is refused rather than
// best-guessed, because "a version field that is read and then ignored is decoration". A v2
// datagram carries no hop, so guessing one would be exactly the defect the field was added to
// remove — the same argument ADR-010 made for the transport.
func TestAVersionTwoAnnouncementIsRefused(t *testing.T) {
	b, err := base(3).Encode()
	if err != nil {
		t.Fatal(err)
	}
	b[offVersion] = 2
	if _, err := Parse(b); err == nil {
		t.Fatal("a version-2 announcement parsed. It carries no hop, so this Nib would have to " +
			"guess which arm it names — which is what the field exists to stop")
	}
}

// TestAnOutOfRangeHopIsRefusedOnBOTHSides — `check` is called by the encoder and the parser, and
// this drives both, because a rule enforced on one side is a rule the two can drift apart on.
func TestAnOutOfRangeHopIsRefusedOnBOTHSides(t *testing.T) {
	if _, err := base(maxHop + 1).Encode(); err == nil {
		t.Error("the encoder accepted a hop it cannot represent, so it would emit an announcement " +
			"it would itself refuse to parse")
	}
	if _, err := base(-2).Encode(); err == nil {
		t.Error("the encoder accepted a negative hop that is not HopNone")
	}

	// And on the parse side, built by hand so the encoder cannot launder it.
	good, err := base(1).Encode()
	if err != nil {
		t.Fatal(err)
	}
	// hopNoneWire is legal; one below it is the largest legal hop. Nothing is out of range on the
	// wire, which is worth stating: the uint16 IS the range, so the parser's job here is only to
	// map the sentinel. This asserts that mapping rather than a refusal that cannot occur.
	binary.BigEndian.PutUint16(good[offHop:offHop+2], hopNoneWire)
	back, err := Parse(good)
	if err != nil {
		t.Fatalf("the sentinel does not parse: %v", err)
	}
	if back.Hop != HopNone {
		t.Errorf("the all-ones hop parsed as %d, want HopNone", back.Hop)
	}
}
