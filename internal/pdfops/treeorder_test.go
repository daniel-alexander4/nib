package pdfops

import (
	"fmt"
	"strings"
	"testing"

	"nib/internal/pdfread"
)

// readingOrder is the tree's document level as a reader walks it: each element's type and the first
// OUTPUT page its content is on.
func readingOrder(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx := readCtx(t, pdf)
	xt := ctx.XRefTable
	cat, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	root := derefDict(xt, cat["StructTreeRoot"])
	if root == nil {
		return "no tree"
	}
	pageNr := map[int]int{}
	for _, pa := range pdfread.Pages(ctx) {
		if pa.Ref != nil {
			pageNr[pa.Ref.ObjectNumber.Value()] = pa.Nr
		}
	}
	_, own := orderLevel(xt, rootKids(xt, root))
	var out []string
	for _, k := range own {
		out = append(out, fmt.Sprintf("%s@%d", nameVal(derefDict(xt, k), "S"), firstPage(xt, k, 0, pageNr, 0, map[int]bool{})))
	}
	return strings.Join(out, " ")
}

// pageOrderedFixture is `subsetFixture` with its document level in page order — the stock fixture reads
// Para@1 P@2 Link@1 …, a link placed after the paragraph of the NEXT page — and, when spanning, with its
// first paragraph running from page 1 onto page 2 as one element.
func pageOrderedFixture(spanning bool) []byte {
	objs := subsetFixtureObjects()
	objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K [32 0 R 34 0 R 33 0 R 35 0 R 36 0 R 37 0 R] >>"
	if spanning {
		objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K [32 0 R 34 0 R 35 0 R 36 0 R 37 0 R] >>"
		objs[32] = "<< /Type /StructElem /S /Para /P 31 0 R /Pg 3 0 R /K [0 << /Type /MCR /Pg 5 0 R /MCID 0 >>] >>"
		objs[39] = "<< /Nums [0 [32 0 R] 1 [32 0 R] 2 [35 0 R] 3 [37 0 R] 4 34 0 R 5 36 0 R] >>"
	}
	return assembleFixture(objs)
}

func collectCarried(t *testing.T, pdf []byte, sel ...string) []byte {
	t.Helper()
	out, err := Collect(pdf, sel)
	if err != nil {
		t.Fatal(err)
	}
	if got := fate(out); got != "carried" {
		t.Fatalf("Collect(%v): fate %q, want carried — the order of a tree that was dropped proves nothing", sel, got)
	}
	return out
}

// TestACarriedTreeIsReadInTheOrderItsPagesComeOut — `/pending 556`. The carry left the tree in the
// SOURCE's order, so a reordered document was read in the order its pages had before the reorder:
// measured before the fix, `Collect(["3","1","2"])` gave first pages 2 2 3 1 1 on this fixture, and
// `Collect(["1","2","1","2"])` of a two-page document 1 1 3 3 2 4.
func TestACarriedTreeIsReadInTheOrderItsPagesComeOut(t *testing.T) {
	src := pageOrderedFixture(false)
	if got, want := readingOrder(t, src), "Para@1 Link@1 P@2 P@3 Link@3 P@4"; got != want {
		t.Fatalf("the fixture reads %q, want %q", got, want)
	}
	// A reorder. Page 3's two elements keep the order they had with each other.
	if got, want := readingOrder(t, collectCarried(t, src, "3", "1", "2")), "P@1 Link@1 Para@2 Link@2 P@3"; got != want {
		t.Errorf("Collect(3,1,2) reads %q, want %q", got, want)
	}
	// A repeat that is not adjacent to what it repeats.
	two := collectCarried(t, src, "3", "4")
	if got, want := readingOrder(t, collectCarried(t, two, "1", "2", "1", "2")), "P@1 Link@1 P@2 P@3 Link@3 P@4"; got != want {
		t.Errorf("Collect(1,2,1,2) reads %q, want %q", got, want)
	}
	// The same-document door is the same selection.
	re, _, err := ReorderPages(src, []string{"3", "1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := readingOrder(t, re), "P@1 Link@1 Para@2 Link@2 P@3"; got != want {
		t.Errorf("ReorderPages(3,1,2) reads %q, want %q", got, want)
	}
	// **An element spanning pages the selection separated or turned round keeps ONE place, by its first
	// output page.** The paragraph runs from the source's page 1 onto its page 2.
	span := pageOrderedFixture(true)
	for _, c := range []struct {
		sel  []string
		want string
	}{
		{[]string{"3", "1", "2", "4"}, "P@1 Link@1 Para@2 Link@2 P@4"}, // kept together
		{[]string{"4", "2", "1", "3"}, "P@1 Para@2 Link@3 P@4 Link@4"}, // turned round: read at page 2's place
		{[]string{"3", "1", "4", "2"}, "P@1 Link@1 Para@2 Link@2 P@3"}, // separated: read at page 1's place
	} {
		if got := readingOrder(t, collectCarried(t, span, c.sel...)); got != c.want {
			t.Errorf("Collect(%v) of a spanning paragraph reads %q, want %q", c.sel, got, c.want)
		}
	}
}

// TestATreeThatWasNotInPageOrderIsLeftAsItWas — the refusal. A wrong re-sort moves elements whose order
// somebody chose: the stock fixture puts page 1's link after page 2's paragraph, so its tree was never in
// page order and a reorder leaves the level exactly as the carry left it.
func TestATreeThatWasNotInPageOrderIsLeftAsItWas(t *testing.T) {
	out := collectCarried(t, subsetFixture(), "3", "1", "2")
	if got, want := readingOrder(t, out), "Para@2 P@3 Link@2 P@1 Link@1"; got != want {
		t.Errorf("Collect(3,1,2) reads %q, want the carry's own order %q", got, want)
	}
}

// TestAnImpositionKeepsTheSourcesReadingOrder — `Booklet`'s sheet order is for the printer. Its selection
// is `Collect`'s with the tree left in source order; ordering by sheet was built once and put the census
// document's H2 before its H1 (ua1 7.4.2 t1, `TestNoOperationAddsAUA1ClauseItsInputDidNotFail`).
func TestAnImpositionKeepsTheSourcesReadingOrder(t *testing.T) {
	src := pageOrderedFixture(false)
	sheet := bookletOrder(4) // 4, 1, 2, 3
	imposed, err := subsetCarrying(src, collectPick(sheet), false, readInSourceOrder)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := readingOrder(t, imposed), "Para@2 Link@2 P@3 P@4 Link@4 P@1"; got != want {
		t.Errorf("the sheet-ordered pages read %q, want the source's own order %q", got, want)
	}
	// Stimulus: the same selection asked for as a reading order does move the last page's element first,
	// so the line above is the flag's doing and not a selection the sort has nothing to say about.
	if got, want := readingOrder(t, collectCarried(t, src, sheet...)), "P@1 Para@2 Link@2 P@3 P@4 Link@4"; got != want {
		t.Errorf("Collect(%v) reads %q, want %q", sheet, got, want)
	}
	// And `Booklet` itself is on the imposition's side of that flag: read off its own output, where the
	// last page shares the first sheet with page 1 and its element is still read last.
	objs := subsetFixtureObjects()
	objs[3] = strings.Replace(objs[3], " /Annots [20 0 R]", "", 1)
	objs[7] = strings.Replace(objs[7], " /Annots [22 0 R]", "", 1)
	objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K [32 0 R 33 0 R 35 0 R 37 0 R] >>"
	objs[32] = "<< /Type /StructElem /S /H1 /P 31 0 R /Pg 3 0 R /K [0] >>"
	objs[37] = "<< /Type /StructElem /S /H2 /P 31 0 R /Pg 9 0 R /K [0] >>"
	objs[39] = "<< /Nums [0 [32 0 R] 1 [33 0 R] 2 [35 0 R] 3 [37 0 R]] >>"
	delete(objs, 20)
	delete(objs, 22)
	delete(objs, 34)
	delete(objs, 36)
	booklet, err := Booklet(assembleFixture(objs), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fate(booklet); got != "carried" {
		t.Fatalf("Booklet: fate %q, want carried — the order of a tree that was dropped proves nothing", got)
	}
	if got, want := readingOrder(t, booklet), "H1@1 P@2 P@2 H2@1"; got != want {
		t.Errorf("the booklet reads %q, want the source's own order %q — its H2 is on the first sheet "+
			"and must still be read after the H1", got, want)
	}
}

// TestAReorderMovesHeadingsOnlyWithTheirPages — what the sort may and may not do to heading nesting. Two
// headings change places only where the selection turned their pages round; that is what was asked for,
// and a document that then starts at an H2 is the honest consequence (as `RemovePages`' row in
// `knownUA1Deltas` is for deleting the page with the only H1).
func TestAReorderMovesHeadingsOnlyWithTheirPages(t *testing.T) {
	objs := subsetFixtureObjects()
	objs[31] = "<< /Type /StructElem /S /Document /P 30 0 R /K [32 0 R 34 0 R 33 0 R 35 0 R 36 0 R 37 0 R] >>"
	objs[32] = "<< /Type /StructElem /S /H1 /P 31 0 R /Pg 3 0 R /K [0] >>"
	objs[35] = "<< /Type /StructElem /S /H2 /P 31 0 R /Pg 7 0 R /K [0] >>"
	src := assembleFixture(objs)
	for _, c := range []struct {
		sel  []string
		want string
	}{
		// Pages 1 and 3 keep their relative order, whatever else moves: H1 stays ahead of H2.
		{[]string{"2", "1", "3"}, "P@1 H1@2 Link@2 H2@3 Link@3"},
		{[]string{"1", "4", "3", "2"}, "H1@1 Link@1 P@2 H2@3 Link@3 P@4"},
		{[]string{"1", "3", "1", "3"}, "H1@1 Link@1 H2@2 Link@2 H1@3 Link@3 H2@4 Link@4"},
		// The user put page 3 ahead of page 1: its heading goes with it.
		{[]string{"3", "1"}, "H2@1 Link@1 H1@2 Link@2"},
	} {
		if got := readingOrder(t, collectCarried(t, src, c.sel...)); got != c.want {
			t.Errorf("Collect(%v) reads %q, want %q", c.sel, got, c.want)
		}
	}
}
