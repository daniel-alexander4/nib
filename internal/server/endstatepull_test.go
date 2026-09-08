package server

import (
	"strings"
	"testing"

	"nib/internal/ceremony"
)

// The fixture is `endedCeremonyFor` in prehopend_test.go — the file that shipped the RECEIVING half
// of this item (a pre-hop party checking an end state that was delivered to it). One fixture for
// both halves, because they are the same ceremony seen from the two sides.

// TestAPreHopPartyReadsTheEndStateItCannotBeDeliveredTo — /pending 380, the round trip.
//
// # The item, in one line
//
// `rearmDeliveries` admits only `LoadOK`, so a party who has accepted and not signed has no
// delivery arm and the convener's round cannot reach them. Admitting `LoadAbsent` was tried and
// backed out: the pre-hop ceremony's arm holds the machine's single delivery slot and the convener
// arrives with a DIFFERENT ceremony, so the arm refuses on its own identity. One slot, many
// ceremonies — so the party PULLS, and a fetch needs no arm.
//
// This drives the two halves that cross the wire: the convener's seal at a target derived from ONE
// party's invitation, and that party's open at the same target. The DHT hop between them is
// `rendezvous`'s and is covered there; what is this item's own is that the two derive the SAME
// target from the same invitation and that the anchor is the invitation rather than a record.
func TestAPreHopPartyReadsTheEndStateItCannotBeDeliveredTo(t *testing.T) {
	inv, _, term := endedCeremonyFor(t, strings.Repeat("ab", 32))
	anchor, err := inv.Anchor()
	if err != nil {
		t.Fatalf("setup: anchor: %v", err)
	}

	// The convener's half.
	seed, err := inv.EndStateSeed()
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv.EndStateSalt()
	if err != nil {
		t.Fatal(err)
	}
	k, err := inv.EndStateKey()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := term.Seal(k, salt, anchor)
	if err != nil {
		t.Fatalf("the convener could not seal its own end state: %v", err)
	}
	if len(seed) == 0 || len(salt) == 0 || len(k) == 0 {
		t.Fatal("setup: an end-state derivation came back empty, so the assertions below are vacuous")
	}

	// The party's half, deriving from the SAME invitation it accepted — which is the whole
	// mechanism: it holds no record, so nothing else could anchor this.
	got, err := ceremony.OpenEndState(k, salt, sealed, anchor)
	if err != nil {
		t.Fatalf("a pre-hop party could not open the end state published for it: %v", err)
	}
	if got.State != ceremony.StateDeclined {
		t.Errorf("the party read state %q, want %q", got.State, ceremony.StateDeclined)
	}

	// ── The refusals, and each is a way this could have ended somebody's ceremony wrongly ──

	// A different ceremony's invitation must not open it. This is the case the whole design
	// turns on: a machine holding several ceremonies must not have one of them ended by another.
	other, _, _ := endedCeremonyFor(t, strings.Repeat("cd", 32))
	if ok, oerr := other.EndStateKey(); oerr == nil {
		osalt, _ := other.EndStateSalt()
		oanchor, _ := other.Anchor()
		if _, err := ceremony.OpenEndState(ok, osalt, sealed, oanchor); err == nil {
			t.Error("another ceremony's invitation opened this end state. Every end-state value " +
				"derives through `Invitation.derive`, which is keyed on the secret, so two " +
				"ceremonies must not share a target — a machine holding several would otherwise " +
				"have one ended by another")
		}
	}

	// The right key at the WRONG salt must not open it: the salt is the AAD, so a record lifted
	// from one target and replayed at another is refused rather than accepted.
	if _, err := ceremony.OpenEndState(k, []byte(strings.Repeat("x", 32)), sealed, anchor); err == nil {
		t.Error("a sealed end state opened at a salt it was not published under. `terminationAAD` " +
			"binds the record to its target precisely so a lifted copy cannot be replayed elsewhere")
	}

	// **The ANCHOR check, on a record that decrypts perfectly.** The refusals above are all
	// enforced by the AEAD — wrong key, wrong salt — so removing `OpenEndState`'s anchor
	// verification left every one of them green. That is /pending 354's trap exactly: a
	// well-formed object that opens and must still be refused because it does not describe THIS
	// proceeding. Same secret and id, so the same key and salt; a different roster hash, so a
	// different anchor.
	wrongAnchorInv := inv
	wrongAnchorInv.RosterHash = strings.Repeat("0", len(inv.RosterHash))
	if wa, waerr := wrongAnchorInv.Anchor(); waerr == nil {
		if _, err := ceremony.OpenEndState(k, salt, sealed, wa); err == nil {
			t.Error("an end state that DECRYPTS at this target was accepted without verifying " +
				"against the invitation anchor. The AEAD only proves it was published by " +
				"somebody holding the ceremony secret; the anchor is what proves it describes " +
				"this proceeding, and it is the check /pending 354 was filed about")
		}
	}

	// And a planted object that does not verify against the invitation is refused — /pending 354's
	// trap, which is why the anchor is the invitation and never a record.json beside it.
	planted := term
	planted.RosterHash = strings.Repeat("0", len(planted.RosterHash))
	if _, perr := planted.Seal(k, salt, anchor); perr == nil {
		t.Error("an end state that does not verify against the invitation was SEALED for " +
			"publication. `Seal` verifies before it encrypts precisely so this machine cannot " +
			"publish an attestation it could not stand behind")
	}
}

// TestTheEndStateTargetIsPerParty — the derivation's own doc claimed otherwise until this landed.
//
// It read "every party reads the same one, and a per-party target would make the convener publish
// N copies". N copies is what the plumbing produces: `derive` is keyed on `i.Secret` and the
// convener takes that secret per party. The comment was corrected rather than the plumbing,
// because a shared BEP-44 target is WRITABLE by everyone who can read it — the private key falls
// out of the same seed — so per-party targets are the stronger property. This pins that.
func TestTheEndStateTargetIsPerParty(t *testing.T) {
	a, _, _ := endedCeremonyFor(t, strings.Repeat("ab", 32))
	b := a
	// Same ceremony, same roster, same id — a DIFFERENT party's secret, which is exactly what
	// `convenerInvitationFor` mints per party.
	b.Secret = append([]byte(nil), a.Secret...)
	b.Secret[0] ^= 0xFF

	as, err := a.EndStateSeed()
	if err != nil {
		t.Fatal(err)
	}
	bs, err := b.EndStateSeed()
	if err != nil {
		t.Fatal(err)
	}
	if string(as) == string(bs) {
		t.Error("two parties of one ceremony derive the SAME end-state seed. A BEP-44 mutable " +
			"target's private key falls out of its seed, so a shared target is one every party " +
			"could overwrite and not merely read")
	}
	asalt, _ := a.EndStateSalt()
	bsalt, _ := b.EndStateSalt()
	if string(asalt) == string(bsalt) {
		t.Error("two parties of one ceremony derive the same end-state salt")
	}
	// And the end-state family is separated from the hop family, or a real hop could collide
	// with it — the rule `derive`'s own doc states.
	hop, err := a.HopSeed(1)
	if err == nil && string(hop) == string(as) {
		t.Error("the end-state seed equals a hop seed, so a hop's target and the end state's are " +
			"the same key: a value used for one purpose being the value used for another")
	}
}
