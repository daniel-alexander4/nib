package pdfops

import (
	"bytes"
	"fmt"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// P02.S02's readers — the n-up carry, repaired.
//
// Every stimulus here was measured before it was written down (v1.129.135): each fixture produces
// the defect its test names, and each one reported `carried` while doing so, which is why nothing
// in this repo could see any of them.

// repeatedPagesFixture is two pages with BYTE-IDENTICAL content, each tagged and each carrying its
// own `/StructParents`.
//
// It exists because identical content is what pdfcpu's optimize pass fuses: after an n-up the two
// sheets' form XObjects compare equal, the optimizer collapses them onto one object, and a single
// `/StructParents` key then has to name two semantic parents. The census document produces the same
// shape at 8 pages over 4 contents; this is the two-page version, hand-built so the reader does not
// depend on a Markdown conversion continuing to repeat itself.
func repeatedPagesFixture() []byte {
	content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (the same words) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R 13 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 13 0 R /K [0] >>",
		13: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 14 0 R /StructParents 1 >>",
		14: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	})
}

// distinctForms counts the form XObjects a document actually holds, and how many of those carrying
// marked content are painted more than once.
func distinctForms(t *testing.T, pdf []byte) (forms, shared int) {
	t.Helper()
	ctx, err := api.ReadAndValidate(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	seen := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			continue
		}
		res, rerr := ctx.DereferenceDict(d["Resources"])
		if rerr != nil || res == nil {
			continue
		}
		eachFormXObject(ctx, res, seen, 0, func(nr int, sd *types.StreamDict) { forms++ })
	}
	for _, d := range formDrawCounts(ctx) {
		if d.count > 1 && d.mcid {
			shared++
		}
	}
	return forms, shared
}

// TestTheCarryGivesEachSheetItsOwnFormXObject — T01, on the shape that produces the fusion.
//
// **The defect is nib's, not pdfcpu's composition.** `api.NUp` writes one form per source page; the
// fusion happens in the optimize pass of a later read, and the carry then anchored the fused object
// once per sheet, overwriting its own `/StructParents` and leaving the other keys owned by nobody.
func TestTheCarryGivesEachSheetItsOwnFormXObject(t *testing.T) {
	src := repeatedPagesFixture()
	if d := completeness(t, src); len(d) > 0 {
		t.Fatalf("setup: the fixture is incomplete before the n-up, so the assertions below would "+
			"pass on a build that does nothing:%s", defectLines(d))
	}
	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fate(out); got != "carried" {
		t.Fatalf("the n-up of a tagged document measures %q, want %q", got, "carried")
	}
	forms, shared := distinctForms(t, out)
	if shared != 0 {
		t.Errorf("%d marked-content form XObject(s) are painted more than once after the carry. One "+
			"`/StructParents` key is one key on one object, so a form drawn on two sheets cannot "+
			"name both — veraPDF scores it 7.20 t2, `isUniqueSemanticParent`", shared)
	}
	if forms < 2 {
		t.Errorf("the carry left %d form XObject(s) for 2 source pages; each placement needs its "+
			"own object to carry its own key", forms)
	}
	if d := completeness(t, out); len(d) > 0 {
		t.Errorf("the n-up output is not completely carried:%s", defectLines(d))
	}
}

// TestTheCarryRepointsAnMCRKid — T03, the kid half of `/pending 503`.
func TestTheCarryRepointsAnMCRKid(t *testing.T) {
	src := mcrFixture()
	if d := completeness(t, src); len(d) > 0 {
		t.Fatalf("setup: the MCR fixture is already incomplete:%s", defectLines(d))
	}
	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if d := completeness(t, out); defectsKeyed(d, "dead-kid-pg") {
		t.Errorf("an MCR kid still names a page that left the page tree. The element was repointed "+
			"and its kid was not, which is the half of the carry that only an MCR-bearing document "+
			"can show:%s", defectLines(d))
	}
}

// TestTheCarryRepointsBothTwins — T02, the visited set keyed on object number.
func TestTheCarryRepointsBothTwins(t *testing.T) {
	out, err := NUp(twinElementFixture(), 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if d := completeness(t, out); defectsKeyed(d, "dead-pg") {
		t.Errorf("one of two byte-identical elements still names a dead page — the walk visited "+
			"them as one. Identity is the object number, never the dictionary's content:%s",
			defectLines(d))
	}
}

// TestBookletCarriesTheTreeToo — T05. `Booklet` is `InsertBlank → Collect → NUp`, so it inherits
// both the composition and every defect of the carry; measured at the grill as 8 form XObjects.
//
// It is NOT expected to carry the tree yet — `Collect` drops it, which is P02.S04 — so this asserts
// the property that must hold either way: whatever Booklet emits, it does not claim a tree it has
// only half repaired.
func TestBookletCarriesTheTreeToo(t *testing.T) {
	src, lerr := LabelUA(labelReady(t, censusMarkdown()), true)
	if lerr != nil {
		t.Fatalf("setup: %v", lerr)
	}
	out, err := Booklet(src, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, shared := distinctForms(t, out); shared != 0 {
		t.Errorf("Booklet leaves %d marked-content form(s) with more than one semantic parent", shared)
	}
	if got := fate(out); got == "orphaned" {
		t.Errorf("Booklet emits a tagging claim over a tree that describes nothing: %v", claims(out))
	}
}

// TestAnIncompleteCarryIsABANDONED — T04, and the reason this door exists as a door.
//
// **Asked of `NUp` this would be vacuous.** Once the carry is repaired no document it composes
// produces an incomplete tree, so a test that drove `NUp` and asserted "the output is complete"
// would be green because the case never arises, not because the gate works. `completeOrHonest` is
// therefore a seam with its own stimulus: a document that really is incompletely carried must come
// back as the honest fallback rather than as a claim.
//
// # The fixture has to be incomplete AND NOT orphaned, and the first cut of this test was neither
//
// It used `sharedFormFixture`, whose element carries no `/Pg` and whose page carries no
// `/StructParents` — which makes the document **orphaned**, and `honest` strips an orphaned claim on
// its own. So the assertion passed with the gate disabled: it was reading `orphaned()`'s work and
// crediting it to the completeness check. The probe caught it, which is what probing each condition
// separately is for. `deadPgFixture("kid")` is the right shape — its element sits on a live page, so
// the document is anchored and `honest` keeps its claim, while its MCR kid names a non-page.
//
// The setup assertion below is the part that must not be dropped: it states, in a form that goes red
// if the fixture ever drifts back, that `honest` alone would NOT drop this claim.
func TestAnIncompleteCarryIsABANDONED(t *testing.T) {
	bad := deadPgFixture("kid")
	if d := completeness(t, bad); len(d) == 0 {
		t.Fatal("setup: the fixture is completely carried, so this drives the wrong branch")
	}
	kept, herr := honest(bad)
	if herr != nil {
		t.Fatal(herr)
	}
	if !inspectTags(kept).claims() {
		t.Fatal("setup: `honest` already drops this document's claim, so the assertion below would " +
			"pass with the completeness gate removed — it would be measuring `orphaned()`, not the " +
			"gate. The fixture must be incomplete and NOT orphaned.")
	}
	raw, err := testpdf.Text("the honest fallback: an untagged document")
	if err != nil {
		t.Fatal(err)
	}
	out, cerr := completeOrHonest(bad, raw)
	if cerr != nil {
		t.Fatal(cerr)
	}
	if inspectTags(out).claims() {
		t.Errorf("a carry that is not complete was shipped with its tagging claim intact. The "+
			"fallback is the honest loss: half a tree under `/Marked true` tells a screen reader "+
			"the document is tagged and then strands it (claims: %v)", claims(out))
	}
}
