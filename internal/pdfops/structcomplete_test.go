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
	for _, who := range owners {
		if strings.HasPrefix(who, "form XObject") {
			form = true
		}
	}
	if !form {
		t.Errorf("after the carry no key is owned by a form XObject (owners: %v). The carry writes "+
			"/StructParents onto the XObject it built from each source page, so a walk that skips "+
			"form XObjects reports a correctly carried document as owning nothing", owners)
	}
}

// TestAnMCRKidLeftBehindByTheCarryIsADefect — condition 2, on the kid rather than the element.
func TestAnMCRKidLeftBehindByTheCarryIsADefect(t *testing.T) {
	src := mcrFixture()
	if d := completeness(t, src); len(d) > 0 {
		t.Fatalf("setup: the MCR fixture is already incomplete before the n-up, so the assertion "+
			"below would pass on a build that does nothing:%s", defectLines(d))
	}
	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fate(out); got != "carried" {
		t.Fatalf("setup: the n-up output measures %q, not %q — this fixture no longer drives the "+
			"case, which is a carry REPORTING success while leaving a kid behind", got, "carried")
	}
	d := completeness(t, out)
	if !defectsKeyed(d, "dead-kid-pg") {
		t.Errorf("the carry left an MCR kid naming a page that is no longer in the page tree, and "+
			"the predicate did not report it. `fate` says %q and `orphaned` is false, so nothing "+
			"else in this repo can see it:%s", fate(out), defectLines(d))
	}
}

// TestATwinElementLeftBehindByTheCarryIsADefect — condition 2, on an element's own `/Pg`.
func TestATwinElementLeftBehindByTheCarryIsADefect(t *testing.T) {
	src := twinElementFixture()
	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	d := completeness(t, out)
	if !defectsKeyed(d, "dead-pg") {
		t.Errorf("the carry left an element naming a dead page and the predicate did not report "+
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

// TestTheCensusNUpIsNotCompletelyCarried — conditions 3 and 4 on the real shape, and the reason
// `NUp` is not yet routed through this predicate.
//
// The census has 8 pages and 4 distinct page contents, so pdfcpu's optimize merges the equal form
// XObjects and the carry binds one XObject to two source pages: measured at the grill as 4 of 8
// `/ParentTree` keys owned by nobody and 2 MCID-bearing forms drawn 3 times each. It reports
// `carried`.
func TestTheCensusNUpIsNotCompletelyCarried(t *testing.T) {
	src, lerr := LabelUA(labelReady(t, censusMarkdown()), true)
	if lerr != nil {
		t.Fatalf("setup: the census document could not be labelled: %v", lerr)
	}
	if d := completeness(t, src); len(d) > 0 {
		t.Fatalf("setup: the census document is incomplete before any operation:%s", defectLines(d))
	}
	out, err := NUp(src, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fate(out); got != "carried" {
		t.Fatalf("setup: the census n-up measures %q — the point of this reader is that a document "+
			"reporting %q is not completely carried", got, "carried")
	}
	d := completeness(t, out)
	if !defectsKeyed(d, "unowned-key") {
		t.Errorf("the census n-up leaves /ParentTree keys nothing claims and the predicate did not "+
			"report it:%s", defectLines(d))
	}
	if !defectsKeyed(d, "shared-form") {
		t.Errorf("the census n-up shares a marked-content form between sheets and the predicate did "+
			"not report it:%s", defectLines(d))
	}
}
