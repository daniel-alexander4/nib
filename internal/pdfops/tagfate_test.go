package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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
// **27 of the 48 declared verdicts were wrong and every one erred the same way.** `Rotate`,
// `Optimize`, `SetLang`, every stamp and every strip carry the tree; the page-set operations drop
// claim and content together, honestly. But the replacement predicate was *"claims tagging and has
// zero elements"*, and **`NUp` emits 45**: they all pointed at pages no longer in the document, and
// no composed sheet carried `/StructParents`. The one operation that genuinely violated law 1 was
// invisible to the predicate written to catch violations of law 1 — and P01.S06 then found that
// even IT was preservable, because the content was intact inside the Form XObjects all along.
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

// TestNUpCARRIESTheTreeRatherThanDroppingIt — P01.S06's first two acceptance clauses.
//
// The phase's exit criterion asks that `nup` not regress veraPDF ua1 7.1 t3 against a tagged input,
// and dropping the claim could never deliver it: 7.1 t3 is about the CONTENT, which stays untagged
// either way, and dropping adds 6.2 t1 and 7.1 t11 on top. Measured on a 4-page LibreOffice
// document after the remap, the n-up output fails **exactly what its input fails** — 5 t1, 7.1 t9,
// 7.1 t10, all producer limitations — and adds nothing.
//
// This is the tier-1 form of that: every element anchored to a page that is in the document, and no
// page with content unreachable from the tree.
func TestNUpCARRIESTheTreeRatherThanDroppingIt(t *testing.T) {
	src := taggedFixture()
	before := inspectTags(src)
	if !before.claims() || before.anchored < 1 {
		t.Fatal("setup: the fixture is not a tagged document, so this asserts nothing")
	}
	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	after := inspectTags(out)
	if after.elements < before.elements {
		t.Errorf("n-up kept %d of the source's %d struct elements. The content survives inside Form "+
			"XObjects with its MCIDs intact, so the tree is re-anchorable and dropping it is a loss "+
			"nothing forced", after.elements, before.elements)
	}
	if after.anchored < 1 {
		t.Errorf("n-up emits %d element(s), none anchored to a page in the document — the remap did "+
			"not run, or it ran and `honest` then dropped the claim", after.elements)
	}
	if after.undescribed != 0 {
		t.Errorf("n-up leaves %d page(s) with content that no struct element points at; a reader "+
			"walking the tree never reaches them", after.undescribed)
	}
	if got := fate(out); got != "carried" {
		t.Errorf("n-up over a tagged input measures %q, want %q", got, "carried")
	}
}

// TestAnUntaggedDocumentIsUNCHANGEDByTheCarry — P01.S06's fourth clause, and the population that is
// nearly every document.
//
// The carry costs a parse of the source before it can know there is nothing to carry. What it must
// never do is *change* an untagged document: the overwhelmingly common case pays a measurement and
// nothing else.
func TestAnUntaggedDocumentIsUNCHANGEDByTheCarry(t *testing.T) {
	src, err := testpdf.Text("an ordinary untagged document")
	if err != nil {
		t.Fatal(err)
	}
	if inspectTags(src).claims() {
		t.Fatal("setup: the plain fixture claims tagging, so this tests the wrong population")
	}
	out, nerr := NUp(src, 2, false)
	if nerr != nil {
		t.Fatal(nerr)
	}
	if s := inspectTags(out); s.claims() {
		t.Errorf("n-up over an UNTAGGED document emits a tagging claim (marked=%v tree=%v). The "+
			"carry must invent nothing", s.marked, s.tree)
	}
	// **And the carry DECLINED rather than merely producing nothing visible.** "Emits no claim" is
	// true of three independent guards at once, so on its own it cannot tell a carry that declined
	// from one that ran over a document it had no business touching. Byte identity would settle it
	// and is unavailable: pdfcpu writes a fresh `/ID` and `/ModDate` on every composition, so two
	// runs of the same input differ by construction (measured — same length, different bytes). So
	// the function is asked directly, on the real production input.
	conf := model.NewDefaultConfiguration()
	nupConf, cerr := api.PDFNUpConfig(2, "border:off, margin:0", conf)
	if cerr != nil {
		t.Fatal(cerr)
	}
	var raw bytes.Buffer
	if e := api.NUp(bytes.NewReader(src), &raw, nil, nil, nupConf, conf); e != nil {
		t.Fatal(e)
	}
	if _, ok := carryTagsThroughNUp(src, raw.Bytes()); ok {
		t.Error("the carry reported success over an untagged source. There is no tree to re-anchor " +
			"and nothing it could have done; reporting success means it would rewrite the " +
			"overwhelmingly common document for no reason, and a rewrite re-encodes")
	}
}

// TestTheCarryIsABANDONEDRatherThanShippedHalfDone — P01.S06's third clause, and the one that keeps
// this from becoming the next version of the mistake this plan is made of.
//
// A partly-anchored tree is worse than an honest loss: some pages reachable, some not, under a
// `/Marked true` claim. `carryTagsThroughNUp` is all-or-nothing by construction, and its caller
// re-measures with `honest` rather than believing the report. This drives the abandonment path
// directly — a document whose pages carry no `/StructParents` gives the carry nothing to match, so
// it must decline and the claim must be dropped.
func TestTheCarryIsABANDONEDRatherThanShippedHalfDone(t *testing.T) {
	src := taggedFixture()
	composed, err := Optimize(src) // a valid document that is NOT an n-up of `src`
	if err != nil {
		t.Fatal(err)
	}
	// Its Form XObjects do not exist, so no page's content can be matched.
	if _, ok := carryTagsThroughNUp(src, composed); ok {
		t.Error("the carry reported success over a document that contains none of the Form XObjects " +
			"it re-anchors. It must decline anything it does not fully understand, because the " +
			"caller's fallback — dropping the claim — is the honest outcome and a half-remapped " +
			"tree is not")
	}
	// And the same input through a source with nothing to carry.
	plain, perr := testpdf.Text("untagged")
	if perr != nil {
		t.Fatal(perr)
	}
	if _, ok := carryTagsThroughNUp(plain, composed); ok {
		t.Error("the carry reported success with no tagged source pages to carry from")
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

// TestAPageWithABOGUSStructParentsCountsAsUndescribed — `/pending 468`'s real finding.
//
// **Merging two TAGGED documents produces the worst state in the package, and the census rated it
// `carried`.** `api.MergeRaw` keeps the first document's `/StructTreeRoot` and `/ParentTree` whole
// while every page of the second keeps its own `/StructParents` value — so an 8-page merge of two
// 4-page documents carries a **4-entry** `/ParentTree` and eight pages indexing keys 0–3 (measured;
// the same shape reproduces here at 2 pages and one entry).
//
// Pages from the second document therefore HAVE a `/StructParents`, and it resolves to structure
// describing entirely different content. Nothing in the tree points at them, so a reader walking it
// from the root never reaches those pages at all — under a document asserting `/Marked true`.
//
// The predicate asked "does this page have `/StructParents`" and so scored the page as described.
// It now asks whether any struct element points AT the page, which is the property that matters and
// which catches the appended-untagged and appended-tagged shapes with one question.
func TestAPageWithABOGUSStructParentsCountsAsUndescribed(t *testing.T) {
	src := taggedFixture()
	both, err := Append(src, src)
	if err != nil {
		t.Fatal(err)
	}
	s := inspectTags(both)
	if s.pages < 2 {
		t.Fatalf("setup: the merge produced %d page(s); with fewer than two there is no second "+
			"document's page to be undescribed and this test asserts nothing", s.pages)
	}
	if s.pagesSP != s.pages {
		t.Fatalf("setup: %d of %d pages carry /StructParents. This test exists for the case where "+
			"EVERY page has one and some of them lie — if pdfcpu has stopped copying the second "+
			"document's /StructParents, the trap this guards is gone and the assertion below is "+
			"passing for a different reason", s.pagesSP, s.pages)
	}
	if s.undescribed == 0 {
		t.Errorf("a merge of two tagged documents reports every page as described: %d pages, all "+
			"with /StructParents, %d element(s), %d anchored. The second document's pages index a "+
			"/ParentTree that does not describe them and no element points at them — they are "+
			"unreachable from the tree under a /Marked true claim, which is law 1's violation "+
			"wearing a valid-looking key.", s.pages, s.elements, s.anchored)
	}
	if got := fate(both); got != "partial" {
		t.Errorf("a merge of two tagged documents measures %q, want %q", got, "partial")
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
