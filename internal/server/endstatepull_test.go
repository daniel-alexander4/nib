package server

import (
	"os"
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

// TestThePreHopEndStateMechanismIsStillWired — /pending 433, and it is a SCAN by necessity.
//
// # What was measured, and why nothing was red
//
// P05's phase close ran a seven-mutation battery over the pre-hop end-state PULL and **every one
// came back green**: the pull never spawned, the pull running after this party has signed, the pull
// skipping the LAN window, not stopping listening, not recording the end state, the round
// publishing to every party rather than the undelivered ones, and the round never publishing at
// all. `fetchEndStateWhenSlow` and `publishEndStateFor` appear in zero test files.
//
// # Why this is a scan and not a behavioural test, established rather than assumed
//
// The entry said the instrument wanted "a fifth party who accepts and never signs" at tier 4. That
// is not what is missing. **`build/pairrepro.sh` says in its own words: "These instances have no
// DHT and no multicast — every hop above is driven by a typed `address=` — so nothing here can
// resolve a rendezvous."** The pull is a `rendezvous.Fetch`, a DHT read with no typed-address
// escape, and the publish is a DHT write; a fifth party in that harness would still have nothing
// for the two halves to meet on.
//
// Tier 1 cannot reach it either: `cer.rz` is a concrete `*rendezvous.Server` and
// `publishEndStateFor` takes a concrete `*sharedRendezvous`, so there is no seam to substitute.
//
// **So the behavioural half is `build/dhtlive.sh`'s, and that is `/pending 2`** — the Instrument
// Missing section's own member, which is exactly "a live-DHT run cannot be had until dhtlive.sh is
// extended with an armed-ceremony case". 433 is deferred onto it.
//
// # What a scan DOES buy here
//
// Five of the seven mutations are DELETIONS — of the spawn, of the window hold, of the record, of
// the teardown, of the publish — and a scan sees a deletion. It cannot see the mechanism working;
// it can see that the mechanism is still there, which is five more than zero. Each clause below
// carries its own stimulus floor, because a scan that cannot find its function reports clean.
func TestThePreHopEndStateMechanismIsStillWired(t *testing.T) {
	session, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := os.ReadFile("delivery.go")
	if err != nil {
		t.Fatal(err)
	}

	// ── The SPAWN, and its guard ─────────────────────────────────────────────
	//
	// The pull belongs to a party who has NOT signed: one who has holds a record and is reachable
	// by the convener's delivery round. Spawning it unconditionally is mutation 2, and it would
	// have every party in a ceremony reading the DHT for an end state the round is about to hand
	// them.
	// `runCeremonyReceive`, not `armCeremonyHop` — the arm's LISTENING half, which is where the
	// pull is spawned and where `cer` is in hand. Named by reading `go s.fetchEndStateWhenSlow(`'s
	// enclosing function rather than by remembering it.
	arm := funcBodyFrom(string(session), strings.Index(string(session), "func (s *Server) runCeremonyReceive("))
	if !strings.Contains(arm, "armAnnouncer") {
		t.Fatal("runCeremonyReceive's body could not be read — an empty body contains none of the " +
			"strings below either, so every clause in this test would pass over nothing")
	}
	if !strings.Contains(arm, "go s.fetchEndStateWhenSlow(") {
		t.Error("the ceremony arm no longer spawns fetchEndStateWhenSlow. A party whose proceeding " +
			"ends before the baton reaches them then holds the interactive slot until the process " +
			"exits, and nothing local ever tells them otherwise")
	}
	if !strings.Contains(arm, "if !cer.hasSigned() {") {
		t.Error("the pull is no longer guarded on this party not having signed. A party who HAS " +
			"signed holds a record and is reached by the delivery round; spawning it for them is " +
			"every party in the ceremony reading the DHT for something they are about to be handed")
	}

	// ── The PULL's own body: the window, the record, the teardown ────────────
	pull := funcBodyFrom(string(delivery), strings.Index(string(delivery), "func (s *Server) fetchEndStateWhenSlow("))
	if !strings.Contains(pull, "cer.rz.Fetch(") {
		t.Fatal("fetchEndStateWhenSlow's body could not be read, or it no longer fetches at all — " +
			"the clauses below would pass over nothing")
	}
	for _, c := range []struct{ needle, why string }{
		{"cer.holdDHT(ctx, hold)",
			"the pull no longer holds the LAN window before reaching the DHT (ADR-011). A " +
				"same-room ceremony would put its first packet on the public network"},
		{"ceremony.WriteTermination(",
			"the pull no longer records the end state it read, so the answer is held in memory " +
				"and the next launch asks again — and /pending 434's ended-check, which reads that " +
				"file, would never see one"},
		{"s.stopListeningFor(",
			"the pull no longer stops listening for a proceeding it has just learned is over, so " +
				"the interactive slot stays taken for a ceremony that has ended"},
		{"ceremony.OpenEndState(",
			"the pull no longer opens the sealed end state against its anchor, which is the only " +
				"thing standing between it and a published object somebody else wrote"},
	} {
		if !strings.Contains(pull, c.needle) {
			t.Errorf("%s (looked for %q in fetchEndStateWhenSlow)", c.why, c.needle)
		}
	}

	// ── The PUBLISH, and that it is only for the legs that did not land ──────
	//
	// Publishing to every party is mutation 6. A publish is off-link traffic under ADR-011, and a
	// party that already holds the document has nothing to read — so the round's own budget clause
	// in pairrepro.sh is measuring a number this would inflate.
	round := funcBodyFrom(string(delivery), strings.Index(string(delivery), "func (s *Server) runDeliveryRound("))
	if !strings.Contains(round, "publishEndStateFor(") {
		t.Fatal("runDeliveryRound's body could not be read, or it no longer publishes the end " +
			"state at all — mutation 7 of /pending 433's battery, and the clause below would " +
			"pass over nothing")
	}
	at := strings.Index(round, "s.publishEndStateFor(")
	before := round[:at]
	if !strings.Contains(before[strings.LastIndex(before, "for _, t := range tasks"):], "Delivered {") {
		t.Error("the end-state publish is no longer inside the not-delivered branch, so the round " +
			"publishes for every party including the ones it just handed the document to. That is " +
			"off-link traffic under ADR-011 for parties with nothing to read, and it inflates the " +
			"packet budget pairrepro.sh grades")
	}
}
