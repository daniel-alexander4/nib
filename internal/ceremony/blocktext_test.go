package ceremony

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/testpdf"
)

// P07.S07a introduced two more strings onto the signature block, and this is the bound that came
// with them.
//
// # Why the bound is part of the slice rather than a follow-up
//
// `IntentFitsBlock` exists because `renderAttestation` draws with `ctx.fillText` and no
// `maxWidth`, so anything wider than the block is silently clipped at the canvas edge — and the
// repo's law is refuse-not-clamp. Before this slice a party's `Label` and `Capacity` were
// carried, committed and never rendered, so their width could not matter. The slice that puts
// them on the block is the slice that owes their bound; shipping without it would have been two
// fresh instances of the defect `IntentFitsBlock`'s own doc comment describes.
//
// Capacity is the one that matters most. It is a claim about a party's AUTHORITY — "as attorney
// under a power of attorney dated 3 June" is an ordinary value and a long one — it is inside the
// signed commitment, and half of it on the page is a document that says something other than what
// the parties agreed.

// blockTextReq is a two-party convene whose second party carries the label and capacity under test.
func blockTextReq(t *testing.T, cfp, afp, label, capacity string) ConveneRequest {
	t.Helper()
	return ConveneRequest{
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		HopBudget:     hopBudget,
		ConvenerSigns: true,
		Roster: []Party{
			{Fingerprint: cfp, Label: "Convener", Signs: true},
			{Fingerprint: afp, Label: label, Capacity: capacity, Signs: true},
		},
	}
}

func TestAnOverWideCapacityIsRefusedAtConveneRatherThanClippedOnTheBlock(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	// SETUP: an ordinary capacity convenes, or the refusal below says nothing about width — it
	// would be a function that refuses every capacity.
	ok := "as Director of Acme Ltd"
	if !p2p.CapacityFitsBlock(ok) {
		t.Fatalf("setup: %q does not fit a block, so this test cannot separate too-wide from "+
			"any-capacity-at-all", ok)
	}
	if _, err := Convene(base, blockTextReq(t, cfp, afp, "A", ok), cert, key, now); err != nil {
		t.Fatalf("an ordinary capacity was refused: %v — an empty or short capacity is the "+
			"common case and must convene", err)
	}

	long := "as attorney under a power of attorney dated 3 June 2019 registered at the Land Registry"
	if p2p.CapacityFitsBlock(long) {
		t.Fatalf("setup: %q fits a block, so this test is not driving the overflow it names", long)
	}
	_, err = Convene(base, blockTextReq(t, cfp, afp, "A", long), cert, key, now)
	if err == nil {
		t.Fatal("a capacity too wide for the block convened anyway: every signature block " +
			"carries it, nothing wraps it, and `ctx.fillText` takes no maxWidth — so it would " +
			"have been silently cut on a document claiming that party's authority")
	}
	if !errors.Is(err, ErrIntentTooLong) {
		t.Errorf("the refusal is %v, which is not the block-overflow error a caller can match on", err)
	}
	// The convener's action is to shorten ONE person's entry, so the sentence must say whose.
	if !strings.Contains(err.Error(), "A") || !strings.Contains(err.Error(), "capacity") {
		t.Errorf("the refusal does not name the party and the field: %q", err.Error())
	}
}

func TestAnOverWideLabelIsRefusedAtConvene(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	long := strings.Repeat("Wilhelmina Fitzgerald-Montmorency ", 4)
	if p2p.LabelFitsBlock(long) {
		t.Fatalf("setup: that label fits a block, so this test is not driving an overflow")
	}
	_, err = Convene(base, blockTextReq(t, cfp, afp, long, ""), cert, key, now)
	if err == nil {
		t.Fatal("a label too wide for the block convened anyway — the block renders " +
			"`Signer: <label>` and would cut it")
	}
	if !errors.Is(err, ErrIntentTooLong) {
		t.Errorf("the refusal is %v, not the block-overflow error", err)
	}
}

// TestTheBlockLineBoundIsOneMeasurement is the ADR-009 half: three fields reach a block line and
// the rule that they must fit is written once.
//
// It compares the three predicates against the SAME underlying measurement rather than against
// each other's literals — two predicates agreeing is the copy agreeing with itself, which is what
// `NominalBlockRect` was written to stop. What it can check cheaply and honestly is that each
// bound moves with its own prefix: a longer prefix leaves room for fewer characters, so
// "Capacity: " must admit strictly fewer runes of the same string than "Signer: " does.
func TestTheBlockLineBoundIsOneMeasurement(t *testing.T) {
	sample := strings.Repeat("m", 400)
	signer := p2p.MaxLabelRunes(sample)      // "Signer: "   — 8 characters
	capacity := p2p.MaxCapacityRunes(sample) // "Capacity: " — 10 characters
	intent := p2p.MaxIntentRunes(sample)     // "Intent: "   — 8 characters

	if signer <= 0 || capacity <= 0 || intent <= 0 {
		t.Fatalf("setup: a bound of zero means nothing fits at all (signer=%d capacity=%d "+
			"intent=%d)", signer, capacity, intent)
	}
	if capacity >= signer {
		t.Errorf("\"Capacity: \" admits %d runes and \"Signer: \" admits %d; the longer prefix "+
			"must leave room for fewer — these two are not measuring the same line",
			capacity, signer)
	}
	if intent != signer {
		t.Errorf("\"Intent: \" admits %d runes and \"Signer: \" admits %d; the two prefixes are "+
			"the same width, so a difference means they are not one measurement", intent, signer)
	}
}
