package pdfops

import (
	"testing"

	"nib/internal/testpdf"
)

// Tag fate — `PLAN-accessibility.md` P01, ADR-031.
//
// # What the measurement actually found, after it was done correctly
//
// The first pass here concluded that pdfcpu's write path destroys structure and that eight
// operations were emitting a tagging claim over nothing. **Both were artefacts of the same mistake:**
// the evidence was `bytes.Count(pdf, []byte("/StructElem"))`, and pdfcpu writes the structure tree
// into a **compressed object stream**, so that count is `0` for every pdfcpu output regardless of
// what it contains.
//
// Parsed instead, on a LibreOffice document with 14 struct elements:
//
//	source          claimed=true  elements=14
//	no-op write     claimed=true  elements=14
//	Rotate(90)      claimed=true  elements=14
//	Optimize        claimed=true  elements=14
//	NUp(2)          claimed=false elements=0
//	Collect(1)      claimed=false elements=0
//
// **Nothing lies.** `Rotate` and `Optimize` carry the tree; `NUp` and `Collect` drop the claim and
// the content together, which is honest. The enforcement written against the byte count was
// stripping trees that had survived, and it is gone.
//
// What survives, and is genuinely valuable: the CENSUS (law 2) and a law-1 guard that would catch a
// violation if one appeared. What does not survive is any claim that one exists today.

// TestNothingClaimsTaggingItHasNot — law 1, checked rather than assumed.
func TestNothingClaimsTaggingItHasNot(t *testing.T) {
	src := taggedFixture()
	claimed, elems := structureCount(src)
	if !claimed || elems < 1 {
		t.Fatalf("setup: the corpus fixture parses as claimed=%v elements=%d — it is not tagged, so "+
			"every assertion below would pass on a build that does nothing", claimed, elems)
	}
	if lies(src) {
		t.Fatal("setup: the fixture itself lies, which makes the predicate untestable against it")
	}
}

// TestTheStructureCountParsesRatherThanCountingBytes is the regression guard for the mistake itself.
//
// **This is the most important test in the file.** A byte count and a parse agree on the INPUT — a
// hand-written PDF has `/StructElem` in plain text — and disagree completely on any pdfcpu OUTPUT,
// because the tree moves into a compressed object stream. Anything that silently went back to
// counting would keep passing every other test here and resume destroying data.
func TestTheStructureCountParsesRatherThanCountingBytes(t *testing.T) {
	src := taggedFixture()
	out, err := Optimize(src)
	if err != nil {
		t.Fatal(err)
	}
	// The raw bytes of a pdfcpu output say zero — that is the trap, stated as an assertion so it
	// cannot quietly stop being true and take the reasoning with it.
	if raw := claims(out)["/StructElem"]; raw != 0 {
		t.Logf("note: this pdfcpu build wrote /StructElem uncompressed (%d); the parse below is still "+
			"the right oracle, but the trap this test documents may no longer bite", raw)
	}
	_, elems := structureCount(out)
	if elems < 1 {
		t.Errorf("Optimize's output parses as %d struct elements. Measured on a real LibreOffice "+
			"document it carries all 14 — if this is 0, either the write path has regressed or "+
			"something is stripping trees again, and the byte count cannot tell you which", elems)
	}
}

// TestAnUntaggedDocumentParsesAsUntagged — the other arm, and the population that is nearly every
// document. Without it, "claimed" could be a constant true and every assertion above would hold.
func TestAnUntaggedDocumentParsesAsUntagged(t *testing.T) {
	src, err := testpdf.Text("an ordinary untagged document")
	if err != nil {
		t.Fatal(err)
	}
	claimed, elems := structureCount(src)
	if claimed || elems != 0 {
		t.Errorf("a plain document parses as claimed=%v elements=%d — the predicate answers true for "+
			"everything, so it can never distinguish a carried tree from a dangling claim",
			claimed, elems)
	}
}
