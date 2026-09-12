package pdfops

import (
	"testing"

	"nib/internal/testpdf"
)

// Tag fate — `PLAN-accessibility.md` P01, ADR-031.
//
// # What the measurement found, after it was done correctly TWICE
//
// The first pass concluded that pdfcpu's write path destroys structure and that eight operations
// emitted a claim over nothing. Both were artefacts of `bytes.Count(pdf, []byte("/StructElem"))`
// against output whose tree is in a compressed object stream.
//
// The correction to that was itself incomplete, and this file records the second finding as
// carefully as the first. Parsed, on a 4-page LibreOffice document with 45 struct elements:
// **19 of 31 operations carry the tree** — `Rotate`, `Optimize`, `SetLang`, every stamp, every
// strip — and the page-set operations drop claim and content together, honestly. But the
// replacement predicate was *"claims tagging and has zero elements"*, and **`NUp` emits 45**: they
// all point at pages that are no longer in the document, and no composed sheet carries
// `/StructParents`. The one operation that genuinely violates law 1 was invisible to the predicate
// written to catch violations of law 1.
//
// veraPDF ua1 confirms it independently: source and `Rotate` fail 5 t1 / 7.1 t9 / 7.1 t10 alike;
// the n-up output adds **7.1 t3, "Content shall be marked as Artifact or tagged as real content",
// 24 failed checks.**

// TestNUpDoesNotClaimTaggingItHasNot is the item's own assertion, and it is the one that could have
// failed: before `honest`, `NUp`'s output was `orphaned` on this very fixture.
func TestNUpDoesNotClaimTaggingItHasNot(t *testing.T) {
	src := taggedFixture()
	s := inspectTags(src)
	if !s.claims() || s.anchored < 1 {
		t.Fatalf("setup: the fixture parses as claims=%v anchored=%d — it is not a tagged document, "+
			"so every assertion below would pass on a build that does nothing", s.claims(), s.anchored)
	}
	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fate(out); got == "orphaned" {
		t.Errorf("NUp emits %s: %v.\n"+
			"Law 1: no output may carry `/MarkInfo /Marked true` or a `/StructTreeRoot` over content "+
			"that is neither tagged nor marked as an artifact. `api.NUp` composes new page objects "+
			"and carries the source catalog onto them, so every struct element points at a page that "+
			"is no longer in the document. veraPDF ua1 scores this as 7.1 t3 with 24 failed checks.",
			got, claims(out))
	}
}

// TestHonestLeavesACarriedTreeALONE is the other arm, and it is the one that matters most.
//
// **The reverted version failed exactly here.** It stripped unconditionally, so a document whose
// tree had survived came out with the tree deleted — real data loss on documents that were fine.
// `honest` is a post-condition and must be a no-op, byte for byte, on anything not orphaned.
func TestHonestLeavesACarriedTreeALONE(t *testing.T) {
	src := taggedFixture()
	for _, tc := range []struct {
		name string
		make func() ([]byte, error)
	}{
		{"the source itself", func() ([]byte, error) { return src, nil }},
		{"Rotate", func() ([]byte, error) { return Rotate(src, nil, 90) }},
		{"Optimize", func() ([]byte, error) { return Optimize(src) }},
	} {
		in, err := tc.make()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if before := inspectTags(in); before.anchored < 1 {
			t.Fatalf("%s: nothing anchored before the door, so this case cannot test what it names", tc.name)
		}
		out, herr := honest(in)
		if herr != nil {
			t.Fatalf("%s: %v", tc.name, herr)
		}
		if len(out) != len(in) || string(out) != string(in) {
			t.Errorf("%s: `honest` rewrote a document it had no business touching (%d bytes in, %d "+
				"out). The unchanged path must not re-encode: that moves bytes and invalidates any "+
				"signature over them", tc.name, len(in), len(out))
		}
		if after := inspectTags(out); after.anchored < 1 {
			t.Errorf("%s: the tree is gone after `honest` — this is the reverted version's defect, "+
				"which destroyed trees that had survived", tc.name)
		}
	}
}

// TestTheOracleParsesRatherThanCountingBytes is the regression guard for the original mistake.
//
// A byte count and a parse agree on the INPUT — a hand-written PDF has `/StructElem` in plain text
// — and disagree completely on any pdfcpu OUTPUT, because the tree moves into a compressed object
// stream. Anything that silently went back to counting would keep passing every other test here and
// resume destroying data.
func TestTheOracleParsesRatherThanCountingBytes(t *testing.T) {
	src := taggedFixture()
	out, err := Optimize(src)
	if err != nil {
		t.Fatal(err)
	}
	if raw := claims(out)["/StructElem"]; raw != 0 {
		t.Logf("note: this pdfcpu build wrote /StructElem uncompressed (%d); the parse below is still "+
			"the right oracle, but the trap this test documents may no longer bite", raw)
	}
	if s := inspectTags(out); s.elements < 1 {
		t.Errorf("Optimize's output parses as %d struct elements. Measured on a real LibreOffice "+
			"document it carries all 45 — if this is 0, either the write path has regressed or "+
			"something is stripping trees again, and the byte count cannot tell you which", s.elements)
	}
}

// TestAnUntaggedDocumentParsesAsUntagged — the population that is nearly every document. Without it,
// `claims` could be a constant true and every assertion above would hold vacuously.
func TestAnUntaggedDocumentParsesAsUntagged(t *testing.T) {
	src, err := testpdf.Text("an ordinary untagged document")
	if err != nil {
		t.Fatal(err)
	}
	s := inspectTags(src)
	if s.claims() || s.elements != 0 {
		t.Errorf("a plain document parses as claims=%v elements=%d — the predicate answers true for "+
			"everything, so it can never distinguish a carried tree from a dangling claim",
			s.claims(), s.elements)
	}
	if fate(src) != "dropped" {
		t.Errorf("a plain document classifies as %q, not %q", fate(src), "dropped")
	}
}

// TestOrphanedRefusesToFireWhileEitherLinkageSurvives pins `orphaned`'s conservatism, which is the
// property standing between this file and a repeat of the data loss.
func TestOrphanedRefusesToFireWhileEitherLinkageSurvives(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    tagState
		want bool
	}{
		{"claim, nothing anchored, no /StructParents", tagState{readable: true, marked: true, tree: true}, true},
		{"claim, an element anchored to a live page", tagState{readable: true, tree: true, anchored: 1}, false},
		{"claim, a page carrying /StructParents", tagState{readable: true, tree: true, pagesSP: 1}, false},
		{"no claim at all", tagState{readable: true, pagesSP: 1}, false},
		{"unreadable", tagState{}, false},
	} {
		if got := tc.s.orphaned(); got != tc.want {
			t.Errorf("%s: orphaned()=%v, want %v", tc.name, got, tc.want)
		}
	}
}
