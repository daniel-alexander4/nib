package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// P02.S01's readers. Every stimulus here was MEASURED at the slice grill before it was written down
// (v1.129.134): each fixture below produces the condition it is named for, and all of them report
// `carried` today, which is the defect the predicate exists to see.

// completeness reads a document and asks the predicate. It fails the test on an unreadable or
// unmodellable document rather than returning "no defects", because "no defects" is exactly what a
// walk that never ran also says.
func completeness(t *testing.T, pdf []byte) []structDefect {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	tree, terr := readStructTree(ctx, live)
	if terr != nil {
		t.Fatalf("the model refuses this tree, so the predicate below would assert nothing: %v", terr)
	}
	return structureCarriedCompletely(ctx, tree)
}

func defectsSay(defects []structDefect, substr string) bool {
	for _, d := range defects {
		if strings.Contains(d.what, substr) {
			return true
		}
	}
	return false
}

// defectsKeyed matches a defect by its KEY rather than its prose.
//
// **Written after the first cut of these tests matched on words and matched the wrong ones.** The
// MCR assertion grepped for "MCR" while the message renders the kid through `kindName()` as
// "marked-content reference", so a correct report read as a miss; and the twin assertion matched
// "not in the page tree", which the MCR message contains too, so it could have passed on a defect
// from the other condition entirely. `structDefect.key` exists for exactly this — it names what is
// broken and is stable against the message being reworded.
func defectsKeyed(defects []structDefect, prefix string) bool {
	for _, d := range defects {
		if strings.HasPrefix(d.key, prefix) {
			return true
		}
	}
	return false
}

func defectLines(defects []structDefect) string {
	var b strings.Builder
	for _, d := range defects {
		b.WriteString("\n  - " + d.String())
	}
	if b.Len() == 0 {
		return " (none)"
	}
	return b.String()
}

// mcrFixture: two pages, where page 2's element reaches its content through an `MCR` kid naming its
// own page. Measured through `NUp(2)`: the carry repoints the ELEMENT's `/Pg` and leaves the MCR
// kid's pointing at the page that no longer exists — `/pending 503`'s defect, which nothing in the
// repo could see because no committed fixture held an MCR.
func mcrFixture() []byte {
	c1 := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (page one) Tj ET\nEMC\n"
	c2 := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (page two a) Tj ET\nEMC\n" +
		"/P <</MCID 1>> BDC\nBT /F1 24 Tf 72 660 Td (page two b) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R 13 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c1), c1),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R 14 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R] 1 [10 0 R 14 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 13 0 R /K [0] >>",
		13: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 15 0 R /StructParents 1 >>",
		14: "<< /Type /StructElem /S /P /P 7 0 R /K [<< /Type /MCR /Pg 13 0 R /MCID 1 >>] >>",
		15: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c2), c2),
	})
}

// sharedFormFixture draws ONE marked-content-bearing form XObject twice on the same page — condition
// 4's minimal shape, built by hand so the reader does not depend on pdfcpu choosing to merge equal
// forms (which is how the census produces it, and which a later pdfcpu could stop doing).
func sharedFormFixture() []byte {
	form := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 0 0 Td (shared) Tj ET\nEMC\n"
	page := "q 1 0 0 1 72 700 cm /Fm0 Do Q\nq 1 0 0 1 72 400 cm /Fm0 Do Q\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Fm0 11 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /K [<< /Type /MCR /Stm 11 0 R /MCID 0 >>] >>",
		9: "<< /Nums [0 [8 0 R]] >>",
		11: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 20] /StructParents 0 "+
			"/Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
	})
}

// TestACompletelyCarriedTreeHasNoDefects is the passing half, and it is the one that makes every
// assertion below mean something: a predicate that refuses everything is indistinguishable from one
// that refuses nothing.
//
// It also carries P02.S01's second acceptance clause — **a key owned by a form XObject passes**.
// After `NUp` the corpus fixture's one `/ParentTree` key is owned by the XObject the carry wrote
// `/StructParents` onto and by nothing else (measured at the grill), so a walk that looked only at
// pages and annotations would report this correct document as owning nothing.
func TestACompletelyCarriedTreeHasNoDefects(t *testing.T) {
	src := taggedFixture()
	if s := inspectTags(src); !s.claims() || s.anchored < 1 {
		t.Fatal("setup: the corpus fixture is not tagged, so this asserts nothing")
	}
	if d := completeness(t, src); len(d) > 0 {
		t.Errorf("the corpus fixture is not completely carried BEFORE any operation:%s", defectLines(d))
	}
	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if d := completeness(t, out); len(d) > 0 {
		t.Errorf("the corpus fixture through NUp(2) reports %d completeness defect(s), and its carry "+
			"is complete — measured at the grill: 0 unowned keys, 0 shared forms, 0 dead /Pg:%s",
			len(d), defectLines(d))
	}
	// The clause in its own right: the key is owned, and the owner is the XObject.
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatal(rerr)
	}
	owners := parentTreeOwners(ctx)
	if len(owners) == 0 {
		t.Fatal("no /ParentTree key is owned by anything after the carry, so condition 3 is vacuous here")
	}
	form := false
	for _, claimants := range owners {
		for _, who := range claimants {
			if strings.HasPrefix(who, "form XObject") {
				form = true
			}
		}
	}
	if !form {
		t.Errorf("after the carry no key is owned by a form XObject (owners: %v). The carry writes "+
			"/StructParents onto the XObject it built from each source page, so a walk that skips "+
			"form XObjects reports a correctly carried document as owning nothing", owners)
	}
}

// deadPgFixture builds a tagged one-page document in which `what` — an element's own `/Pg`, or its
// MCR kid's — names object 5, the font. Object 5 exists and is not a page, so the reference is
// exactly what a carry leaves behind when it repoints some references and not others.
//
// **These were driven through `NUp` until P02.S02 repaired the carry**, and that is why they are
// built by hand now: the operation no longer produces either defect, so a test that still drove it
// would assert nothing and pass for the wrong reason. The predicate's job is to SEE a dead `/Pg`,
// and that is what these two ask, on documents that have one.
func deadPgFixture(what string) []byte {
	content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (words) Tj ET\nEMC\n"
	elem := "<< /Type /StructElem /S /P /P 7 0 R /Pg 5 0 R /K [0] >>"
	if what == "kid" {
		elem = "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [<< /Type /MCR /Pg 5 0 R /MCID 0 >>] >>"
	}
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8: elem,
		9: "<< /Nums [0 [8 0 R]] >>",
	})
}

// TestAnMCRKidNamingADeadPageIsADefect — condition 2, on the kid rather than the element.
func TestAnMCRKidNamingADeadPageIsADefect(t *testing.T) {
	src := deadPgFixture("kid")
	if got := fate(src); got == "orphaned" {
		t.Fatalf("setup: the fixture is orphaned, so `orphaned()` already catches it and this "+
			"asserts nothing about the completeness predicate (fate %q)", got)
	}
	d := completeness(t, src)
	if !defectsKeyed(d, "dead-kid-pg") {
		t.Errorf("an MCR kid names an object that is not a page in the page tree, and the predicate "+
			"did not report it. `fate` says %q and `orphaned` is false, so nothing else in this "+
			"repo can see it:%s", fate(src), defectLines(d))
	}
}

// sharedKeyFixture is two pages carrying the SAME `/StructParents`, with one row of elements between
// them — what `DuplicatePage` produces once a subset carries its tree, since it is
// `Collect(pdf, ["1-p", "p-"])` and `Collect` preserves a repeated page.
func sharedKeyFixture() []byte {
	content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (same) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R 13 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9: "<< /Nums [0 [8 0 R]] >>",
		13: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> " +
			"/Contents 14 0 R /StructParents 0 >>",
		14: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	})
}

// TestTwoPagesSharingOneParentTreeKeyIsADefect — condition 5, and the hole S04 would have walked into.
//
// **Measured before it was written down**: `parentTreeOwners` answered `map[0:page 1]` on this
// fixture while two pages claimed key 0, `checkStructConsistency` reported nothing, and
// `structureCarriedCompletely` reported nothing — so a duplicated page under a carried tree would
// have shipped with one row of elements describing one of its two claimants.
func TestTwoPagesSharingOneParentTreeKeyIsADefect(t *testing.T) {
	src := sharedKeyFixture()
	if got := fate(src); got == "orphaned" {
		t.Fatalf("setup: the fixture is orphaned, so `orphaned()` catches it and this says nothing "+
			"about the completeness predicate (fate %q)", got)
	}
	d := completeness(t, src)
	if !defectsKeyed(d, "shared-key") {
		t.Errorf("two pages claim /ParentTree key 0 and the predicate did not report it. One key "+
			"names one row of elements: the elements describe one page, and the other page's "+
			"content is reached through references naming its twin:%s", defectLines(d))
	}
}

// TestAnElementNamingADeadPageIsADefect — condition 2, on an element's own `/Pg`.
func TestAnElementNamingADeadPageIsADefect(t *testing.T) {
	src := deadPgFixture("elem")
	d := completeness(t, src)
	if !defectsKeyed(d, "dead-pg") {
		t.Errorf("an element names an object that is not a page and the predicate did not report "+
			"it:%s", defectLines(d))
	}
}

// TestAFormDrawnTwiceUnderMarkedContentIsADefect — condition 4, on a hand-built shape.
func TestAFormDrawnTwiceUnderMarkedContentIsADefect(t *testing.T) {
	src := sharedFormFixture()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(src), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus before response: the form really is painted twice and really carries MCIDs.
	draws := formDrawCounts(ctx)
	if len(draws) != 1 {
		t.Fatalf("setup: %d form XObject(s) counted, want exactly 1 — the fixture is not driving "+
			"the case", len(draws))
	}
	for nr, d := range draws {
		if d.count != 2 || !d.mcid {
			t.Fatalf("setup: form %d is drawn %d time(s), mcid=%v; the fixture must paint one "+
				"marked-content form twice", nr, d.count, d.mcid)
		}
	}
	if d := completeness(t, src); !defectsKeyed(d, "shared-form") {
		t.Errorf("one marked-content form painted twice has two semantic parents (veraPDF 7.20 t2) "+
			"and the predicate did not report it:%s", defectLines(d))
	}
}

// TestTheCensusNUpIsCompletelyCarried — P02.S02's acceptance, in tier-1 form.
//
// **This test asserted the opposite until S02 landed**, and the inversion is the slice. The census
// has 8 pages over 4 distinct page contents, so pdfcpu's optimize pass fused the equal form
// XObjects and the carry anchored one object once per sheet: 4 of 8 `/ParentTree` keys owned by
// nobody and 2 MCID-bearing forms drawn 3 times each, all of it reported `carried`. The carry
// un-fuses now, so the real document composes to a tree that is complete — and veraPDF agrees,
// which is what the removal of `knownUA1Deltas`' `NUp` row asserts one file over.
func TestTheCensusNUpIsCompletelyCarried(t *testing.T) {
	src, lerr := LabelUA(labelReady(t, censusMarkdown()), true)
	if lerr != nil {
		t.Fatalf("setup: the census document could not be labelled: %v", lerr)
	}
	if d := completeness(t, src); len(d) > 0 {
		t.Fatalf("setup: the census document is incomplete before any operation:%s", defectLines(d))
	}
	if n, perr := PageCount(src); perr != nil || n < 3 {
		t.Fatalf("setup: the census has %d page(s) (err %v); the fusion needs several sheets to show", n, perr)
	}
	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fate(out); got != "carried" {
		t.Fatalf("the census n-up measures %q, want %q — a carry that is abandoned drops the claim, "+
			"which is honest but is not what a repaired carry should do here", got, "carried")
	}
	if d := completeness(t, out); len(d) > 0 {
		t.Errorf("the census n-up is not completely carried:%s", defectLines(d))
	}
}
