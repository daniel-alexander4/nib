package pdfops

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
)

// flatTaggedDoc is n pages under ONE `/Pages` — the flat tree pdfcpu's writer and nib's Markdown conversion
// produce — each page drawing one marked paragraph that its own element owns through `/ParentTree`. The pages
// share their content stream and font, so the per-page work the gate does is as small as a tagged page allows
// and the page-tree walk is what the timing sees.
func flatTaggedDoc(n int) []byte {
	content := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 700 Td (Clause) Tj ET\nEMC\n"
	pagesAt, elemsAt := 10, 10+n
	objs := map[int]string{
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var kids, elems, nums strings.Builder
	for i := 0; i < n; i++ {
		pg, el := pagesAt+i, elemsAt+i
		objs[pg] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> "+
			"/Contents 4 0 R /StructParents %d >>", i)
		objs[el] = fmt.Sprintf("<< /Type /StructElem /S /P /P 7 0 R /Pg %d 0 R /K [0] >>", pg)
		fmt.Fprintf(&kids, "%d 0 R ", pg)
		fmt.Fprintf(&elems, "%d 0 R ", el)
		fmt.Fprintf(&nums, "%d [%d 0 R] ", i, el)
	}
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>"
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), n)
	objs[7] = fmt.Sprintf("<< /Type /StructTreeRoot /K [%s] /ParentTree 9 0 R /ParentTreeNextKey %d >>", elems.String(), n)
	objs[9] = fmt.Sprintf("<< /Nums [%s] >>", nums.String())
	return assembleFixture(objs)
}

// TestTheCarryGateIsLinearInPagesOnAFlatTree — `/pending 754`. The gate re-read the carried output through
// pdfcpu's whole optimize pass, whose `optimizeResourceDicts` asks `PageDict(i, true)` for every page, and
// `PageDict` walks a flat tree from its root each time: quadratic, ~15 s of a 7,059-page document's ~23 s
// prepare. The gate now reads without that one step (see `carryIsComplete`), so four times the pages must cost
// nowhere near sixteen times the time.
func TestTheCarryGateIsLinearInPagesOnAFlatTree(t *testing.T) {
	gate := func(n int) time.Duration {
		pdf := flatTaggedDoc(n)
		// STIMULUS: a tagged document the gate answers complete — the whole gate runs, not an early false.
		if !carryIsComplete(pdf) {
			t.Fatalf("setup: the gate refused the %d-page flat tagged fixture", n)
		}
		best := time.Duration(1 << 62)
		for i := 0; i < 5; i++ {
			st := time.Now()
			carryIsComplete(pdf)
			best = min(best, time.Since(st))
		}
		return best
	}
	const n = 1500
	small, large := gate(n), gate(4*n)
	ratio := float64(large) / float64(small)
	t.Logf("%d pages %v, %d pages %v: %.1fx for 4x the pages", n, small, 4*n, large, ratio)
	if ratio > 8 {
		t.Errorf("the carry gate is superlinear in pages on a flat tree: %d pages took %.1fx the time of %d "+
			"(%v vs %v) — its re-read walks the page tree once per page", 4*n, ratio, n, large, small)
	}
}

// TestTheCarryGateSeesAFormDrawnTwiceThroughInheritedResources — the half of `optimizeResourceDicts` the gate
// still needs (`/pending 754`). A page with no `/Resources` of its own draws from its `/Pages` node's; skipping
// the step left that page's resources nil, so its draws went uncounted and a marked form painted twice —
// condition 4 — passed.
func TestTheCarryGateSeesAFormDrawnTwiceThroughInheritedResources(t *testing.T) {
	pdf := inheritedTwiceFixture()
	if carryIsComplete(pdf) {
		t.Error("a marked form drawn twice from inherited /Resources passed the carry gate")
	}
	// Incomplete is not enough: without the inherited resources the page owns nothing either, so the gate
	// still refuses — on an unowned key, for the wrong reason. The draw count must see the form twice.
	ctx, err := pdfread.ReadForInspection(pdf)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := readStructTree(ctx, livePageObjects(ctx))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, d := range structureCarriedCompletely(ctx, tree) {
		keys = append(keys, d.key)
	}
	if want := "shared-form obj=6"; strings.Join(keys, "|") != want {
		t.Errorf("the gate's reading reported %q, want exactly %q — the page's inherited /Resources were not "+
			"read, so its two draws of the marked form went uncounted", keys, want)
	}
}

// inheritedTwiceFixture is one page, no `/Resources` of its own, painting its `/Pages` node's marked form twice.
func inheritedTwiceFixture() []byte {
	form := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 0 0 Td (Twice) Tj ET\nEMC\n"
	page := "q /Fm Do Q q 1 0 0 1 0 300 cm /Fm Do Q\n"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 /Resources << /XObject << /Fm 6 0 R >> /Font << /F1 5 0 R >> >> >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> "+
			"/StructParents 0 /Length %d >>\nstream\n%s\nendstream", len(form), form),
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [<< /Type /MCR /Pg 3 0 R /Stm 6 0 R /MCID 0 >>] >>",
		9: "<< /Nums [0 [8 0 R]] >>",
	}
	return assembleFixture(objs)
}
