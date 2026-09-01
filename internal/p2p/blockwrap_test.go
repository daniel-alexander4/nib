package p2p

import (
	"strings"
	"testing"
	"time"

	"nib/mdpdf"
)

// TestABlockLineWraps is /pending 286's core: a value too wide for one line takes as many as it
// needs instead of being refused.
//
// Before this, `AppearanceLines` emitted one entry per field and the four user-supplied strings
// each had a one-line ceiling — so a recital of ordinary length for a lease was refused outright,
// and the alternative the refusal protected against was `ctx.fillText` clipping it at the canvas
// edge with no `maxWidth`.
func TestABlockLineWraps(t *testing.T) {
	long := "We agree to be bound by the terms of the lease dated 1 September 2026 between the " +
		"parties named herein, and to perform every covenant it contains."
	// STIMULUS: it really does not fit one line, or "it wrapped" is a claim about nothing.
	if blockLineFits("Intent: ", long) {
		t.Fatal("setup: the fixture fits on one line, so this test cannot show wrapping")
	}
	lines := wrapBlockLine("Intent: ", long)
	if len(lines) < 2 {
		t.Fatalf("a recital wider than the block rendered on %d line(s) — it was not wrapped, so "+
			"the client would clip it at the canvas edge", len(lines))
	}
	// EVERY line must fit, which is the property that makes wrapping a fix rather than a rename.
	for i, ln := range lines {
		if mdpdf.CoreWidth(ln, readmeFont, blockTextPt) > blockTextWidth() {
			t.Errorf("wrapped line %d is wider than the block: %q", i, ln)
		}
	}
	// And nothing is lost. Joining on a space must reproduce the original text.
	if got := strings.Join(lines, " "); got != "Intent: "+long {
		t.Errorf("wrapping changed the text.\n got %q\nwant %q", got, "Intent: "+long)
	}
}

// TestAnUnbreakableTokenIsHardBroken is the case a greedy wrapper silently gets wrong.
//
// A 400-character word has no space to break at. A wrapper that only splits on spaces emits ONE
// line and the canvas clips it — which is the exact silent truncation this whole bound exists to
// prevent, reintroduced inside the thing meant to fix it. It is also the termination argument: a
// loop that cannot consume a rune when nothing fits does not finish.
func TestAnUnbreakableTokenIsHardBroken(t *testing.T) {
	token := strings.Repeat("Z", 400)
	done := make(chan []string, 1)
	go func() { done <- wrapBlockLine("Signer: ", token) }()
	var lines []string
	select {
	case lines = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("wrapBlockLine did not terminate on a token with no spaces — the fallback that " +
			"guarantees at least one rune per iteration is gone")
	}
	if len(lines) < 2 {
		t.Fatalf("a 400-character unbreakable token rendered on %d line(s); it must be hard-broken, "+
			"or the canvas clips it exactly as it did before wrapping existed", len(lines))
	}
	for i, ln := range lines {
		if mdpdf.CoreWidth(ln, readmeFont, blockTextPt) > blockTextWidth() {
			t.Errorf("hard-broken line %d still overflows the block: %d runes", i, len([]rune(ln)))
		}
	}
	// No character is lost or duplicated. Spaces are compared out because a break AT a space
	// legitimately consumes it — what must survive is every other rune, in order.
	strip := func(x string) string { return strings.ReplaceAll(x, " ", "") }
	if got, want := strip(strings.Join(lines, "")), strip("Signer: "+token); got != want {
		t.Errorf("hard-breaking lost or duplicated characters: %d runes out, %d in",
			len([]rune(got)), len([]rune(want)))
	}
	// And it does not waste a line: breaking at the space after the prefix would spend a whole
	// line on seven characters, and on a block whose height IS the legibility budget that costs
	// every other line a smaller font.
	if len([]rune(lines[0])) < 20 {
		t.Errorf("the first line is %q — %d runes. The wrapper broke at the space after the "+
			"prefix instead of hard-breaking, spending a line to render almost nothing",
			lines[0], len([]rune(lines[0])))
	}
}

// TestTheFixedBlockLinesFit guards the three lines wrapBlockLine is never asked about.
//
// The header, `Party k of n` and `Time:` are emitted verbatim on the assumption that they always
// fit. That assumption is load-bearing — if one of them overflowed, the block would clip a line
// nothing measures — and it is cheap to check rather than assume.
func TestTheFixedBlockLinesFit(t *testing.T) {
	for _, s := range []string{
		"Nib co-signing attestation",
		"Party 99 of 99",
		"Time: 2026-09-01 12:00 MST",
	} {
		if mdpdf.CoreWidth(s, readmeFont, blockTextPt) > blockTextWidth() {
			t.Errorf("the fixed block line %q does not fit; nothing wraps it, so it would be "+
				"clipped at the canvas edge", s)
		}
	}
}

// TestBlockOverflowNamesTheWorstFieldAndACuttableNumber — the refusal's content.
//
// "Shorten something" is not an instruction. The refusal has to say WHICH field and HOW MUCH, and
// the number has to be one the user can actually cut to: `fits` is computed with the other fields
// held as they are, which is the question someone editing one box actually has.
func TestBlockOverflowNamesTheWorstFieldAndACuttableNumber(t *testing.T) {
	a := Attestation{
		Signer:   "A",
		Capacity: "as Director",
		Intent:   strings.Repeat("We agree to grant a lease of Flat 3 Acacia Avenue ", 12),
		Position: 1, RosterSize: 2,
	}
	if BlockFits(a) {
		t.Fatalf("setup: the fixture fits in %d lines, so there is no overflow to describe",
			BlockLineCount(a))
	}
	lines, limit, worst, fits := BlockOverflow(a)
	if lines <= limit {
		t.Errorf("BlockOverflow reports %d lines against a limit of %d — it is describing a block that fits", lines, limit)
	}
	if worst != "the recital" {
		t.Errorf("the longest field is reported as %q; the recital is twelve repeats and every "+
			"other field is a few words, so a refusal naming anything else sends the user to "+
			"edit the wrong box", worst)
	}
	// THE NUMBER MUST BE ACTIONABLE, in both directions — this is the property that makes it a
	// number rather than a decoration.
	cut := a
	cut.Intent = string([]rune(a.Intent)[:fits])
	if !BlockFits(cut) {
		t.Errorf("the refusal quotes %d characters and cutting to exactly that still does not "+
			"fit — a user who follows the instruction is refused again", fits)
	}
	if fits < len([]rune(a.Intent)) {
		more := a
		more.Intent = string([]rune(a.Intent)[:fits+1])
		if BlockFits(more) {
			t.Errorf("one character past the quoted %d still fits, so the number is not tight "+
				"and the user is told to cut more than they need", fits)
		}
	}
}

// TestAShortBlockIsUnchangedByWrapping is the compatibility claim, asserted rather than argued.
//
// /pending 286 rested on a measurement: wrapping does not change what existing blocks render as,
// because anything at or under six lines is what they already were. If that stops being true, every
// document Nib has produced starts rendering differently from the ones beside it.
func TestAShortBlockIsUnchangedByWrapping(t *testing.T) {
	a := Attestation{
		Signer: "Ada Lovelace", AcceptedPeerLabel: "Acme Ltd",
		AcceptedPeer: strings.Repeat("ab", 32),
		Intent:       "I accept", When: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
	lines := a.AppearanceLines()
	want := []string{
		"Nib co-signing attestation",
		"Signer: Ada Lovelace",
		"Accepts: Acme Ltd  [" + shortFingerprint(a.AcceptedPeer) + "]",
		"Intent: I accept",
		"Time: 2026-09-01 12:00 UTC",
	}
	if len(lines) != len(want) {
		t.Fatalf("an ordinary manual block renders %d lines, want %d — %q", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d is %q, want %q — wrapping changed a block that already fit", i, lines[i], want[i])
		}
	}
}

// TestTheBlockHeightHonoursTheLegibilityFloor ties `maxBlockLines` to the thing it was chosen for.
//
// The constant is a line count, but the DECISION was a font size: Dan set the floor at 8 lines on
// 2026-09-01 because that renders at 6.65pt on a 280x84pt block, and below about 6pt a signature
// block on a legal instrument stops being readable in print. A test that merely restated the
// number would be the constant agreeing with itself; this asserts the property the number encodes,
// so raising the cap fails here rather than silently shipping smaller text.
//
// The formula is the CLIENT's, mirrored from `web/app.js` renderAttestation:
//
//	lineH = (height*scale - 2*pad) / lines ; font = min(lineH*0.7, 9*scale) ; pt = font/scale
func TestTheBlockHeightHonoursTheLegibilityFloor(t *testing.T) {
	const (
		scale      = 3.0
		pad        = 4 * scale
		floorPt    = 6.5 // below this a block on a signed instrument is not print-legible
		blockCapPt = 9.0
	)
	r := NominalBlockRect()
	h := (r[3] - r[1]) * scale
	lineH := (h - 2*pad) / float64(maxBlockLines)
	pt := lineH * 0.7 / scale
	if pt > blockCapPt {
		pt = blockCapPt
	}
	if pt < floorPt {
		t.Errorf("a block at the %d-line limit renders at %.2fpt, below the %.1fpt floor. The "+
			"limit is a legibility decision expressed as a line count — raising it makes every "+
			"full block smaller, on documents that are already signed and distributed by the "+
			"time anyone notices", maxBlockLines, pt, floorPt)
	}
	// And the limit is not so tight that it refuses what it was widened to admit: a recital of
	// ordinary length for a lease must fit beside a name and a capacity.
	ok := Attestation{
		Signer: "Wendy Okonkwo", Capacity: "as Director of Okonkwo & Reyes LLP",
		Intent:   "We agree to be bound by the terms of the lease dated 1 September 2026.",
		Position: 3, RosterSize: 9,
	}
	if !BlockFits(ok) {
		t.Errorf("an ordinary lease recital beside a real firm name needs %d lines and the limit "+
			"is %d — the bound is too tight to convene real work, which is what /pending 286 "+
			"existed to fix", BlockLineCount(ok), maxBlockLines)
	}
}
