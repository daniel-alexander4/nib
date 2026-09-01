package ceremony

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/testpdf"
)

// The block-size rule, and it is JOINT since /pending 286 (2026-09-01).
//
// # What replaced what
//
// P07.S07a put a party's Label and Capacity on the block and bounded each to ONE LINE, alongside
// the recital, because `renderAttestation` draws with `ctx.fillText` and no `maxWidth` and the
// repo's law is refuse-not-clamp. Block lines wrap now, so the one-line ceilings are gone and the
// question is the block's HEIGHT: at most `maxBlockLines`, which is a legibility floor expressed
// as a line count (8 lines = 6.65pt on a 280x84pt block).
//
// **The three fields therefore stopped being independent, and that is the whole change.** A
// recital that fits beside a short capacity does not fit beside a long one, so the rule can only
// be asked of a whole block — per SIGNING party, since a non-signing party has no block. These
// tests assert the joint rule and the sentence it produces, because "shorten something" is not an
// instruction a convener can act on.
//
// Capacity is still the field that matters most: it is a claim about a party's AUTHORITY, it is
// inside the signed commitment, and half of it on the page is a document that says something other
// than what the parties agreed.

// blockTextReq is a two-party convene whose second party carries the label and capacity under test.
func blockTextReq(t *testing.T, cfp, afp, label, capacity string) ConveneRequest {
	t.Helper()
	return ConveneRequest{
		Intent:         "We agree to co-sign the lease",
		Expires:        time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		HopBudget:      hopBudget,
		DeliveryBudget: hopBudget,
		ConvenerSigns:  true,
		Roster: []Party{
			{Fingerprint: cfp, Label: "Convener", Signs: true},
			{Fingerprint: afp, Label: label, Capacity: capacity, Signs: true},
		},
	}
}

func TestAnOverLongCapacityIsRefusedAtConveneRatherThanShrunkPastLegibility(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	// SETUP: a capacity that WRAPS is fine now, and asserting that first is what keeps the
	// refusal below about height rather than about "any capacity at all". This exact string was
	// REFUSED before /pending 286 — it is 87 characters and does not fit one line.
	wraps := "as attorney under a power of attorney dated 3 June 2019 registered at the Land Registry"
	att := p2p.Attestation{Signer: "A", Capacity: wraps, Intent: "We agree to co-sign the lease",
		Position: 1, RosterSize: 2}
	if n := p2p.BlockLineCount(att); n <= 1 {
		t.Fatalf("setup: that capacity renders on %d line(s); it is meant to wrap, so this test "+
			"is not exercising what it claims", n)
	}
	if !p2p.BlockFits(att) {
		t.Fatalf("setup: a wrapping capacity does not fit at all (%d lines) — the refusal below "+
			"would then be about any capacity rather than an over-long one",
			p2p.BlockLineCount(att))
	}
	if _, err := Convene(base, blockTextReq(t, cfp, afp, "A", wraps), cert, key, now); err != nil {
		t.Fatalf("a capacity that wraps within the block was refused: %v — this is precisely the "+
			"value /pending 286 exists to admit", err)
	}

	// Now one that cannot fit however it wraps.
	long := strings.Repeat("as attorney under a power of attorney dated 3 June 2019, ", 4)
	over := p2p.Attestation{Signer: "A", Capacity: long, Intent: "We agree to co-sign the lease",
		Position: 1, RosterSize: 2}
	if p2p.BlockFits(over) {
		t.Fatalf("setup: that capacity fits in %d lines, so this test is not driving the "+
			"overflow it names", p2p.BlockLineCount(over))
	}
	_, err = Convene(base, blockTextReq(t, cfp, afp, "A", long), cert, key, now)
	if err == nil {
		t.Fatal("a capacity too long for the block convened anyway: every signature block " +
			"carries it, and the client shrinks the text as the line count rises — so it would " +
			"have rendered below the legibility floor on a document claiming that party's " +
			"authority")
	}
	if !errors.Is(err, ErrIntentTooLong) {
		t.Errorf("the refusal is %v, which is not the block-overflow error a caller can match on", err)
	}
	// The convener's action is to shorten ONE person's entry, so the sentence must say whose AND
	// which field — "the block is too big" sends them to re-read the whole roster.
	if !strings.Contains(err.Error(), "A") || !strings.Contains(err.Error(), "capacity") {
		t.Errorf("the refusal does not name the party and the longest field: %q", err.Error())
	}
}

// TestTheBlockBudgetIsJOINT is the property the per-field bounds could not express.
//
// Two values that are each individually admissible combine into a block that is not. Before
// /pending 286 this could not even be stated: each field had its own one-line ceiling and nothing
// asked about their sum.
func TestTheBlockBudgetIsJOINT(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	capacity := "as attorney under a power of attorney dated 3 June 2019 registered at the Land Registry"
	recital := "The parties agree to be bound by the terms of the lease dated 1 September 2026 and to " +
		"perform every covenant and condition it contains without reservation."

	// EACH ALONE FITS — asserted, not assumed, or the joint failure below proves nothing.
	alone := func(cap, rec string) p2p.Attestation {
		return p2p.Attestation{Signer: "A", Capacity: cap, Intent: rec, Position: 1, RosterSize: 2}
	}
	shortRecital := "We agree to co-sign the lease"
	if !p2p.BlockFits(alone(capacity, shortRecital)) {
		t.Fatalf("setup: the capacity alone does not fit (%d lines)", p2p.BlockLineCount(alone(capacity, shortRecital)))
	}
	if !p2p.BlockFits(alone("", recital)) {
		t.Fatalf("setup: the recital alone does not fit (%d lines)", p2p.BlockLineCount(alone("", recital)))
	}

	// TOGETHER they do not.
	both := alone(capacity, recital)
	if p2p.BlockFits(both) {
		t.Skipf("this fixture no longer overflows when combined (%d lines); the geometry moved "+
			"and the pair must be re-chosen for this test to mean anything", p2p.BlockLineCount(both))
	}

	req := blockTextReq(t, cfp, afp, "A", capacity)
	req.Intent = recital
	_, err = Convene(base, req, cert, key, now)
	if err == nil {
		t.Fatal("a capacity and a recital that each fit alone convened together into a block " +
			"that does not. That is the whole reason the bound is joint: a per-field ceiling " +
			"admits both and the block it produces renders below the legibility floor.")
	}
	if !errors.Is(err, ErrIntentTooLong) {
		t.Errorf("the refusal is %v, not the block-overflow error", err)
	}
}

// TestTheBlockBoundIsOneDoor is the ADR-009 half: the height rule is written once and every site
// that can ask it asks THAT.
//
// It asserts ROUTING rather than comparing the sentences each site prints — eight copies checked
// for agreement say nothing about a ninth site added without one, which is ADR-009's own wording.
func TestTheBlockBoundIsOneDoor(t *testing.T) {
	src, err := os.ReadFile("convene.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)
	if !strings.Contains(code, "p2p.BlockFits(") {
		t.Error("convene.go does not route through p2p.BlockFits — the block-height rule would " +
			"then be enforced somewhere else, or not at all, on the door where the convener is " +
			"still able to retype")
	}
	// And the per-field ceilings really are gone, or two rules police one thing and disagree.
	for _, dead := range []string{"IntentFitsBlock", "CapacityFitsBlock", "LabelFitsBlock"} {
		if strings.Contains(code, "p2p."+dead+"(") {
			t.Errorf("convene.go still calls p2p.%s — a one-line ceiling beside the joint "+
				"height bound refuses values the block can now render, which is the defect "+
				"/pending 286 closed", dead)
		}
	}
}
