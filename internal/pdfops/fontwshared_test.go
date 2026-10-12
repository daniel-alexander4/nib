package pdfops

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"nib/internal/pdfread"
)

// pagesSharingACIDFont is n pages that each show two glyphs of ONE Type0 font, whose descendant is an indirect
// object (as written) or a direct dictionary. `/W` gives CID 0x41 and 0x42 advances of 250 and 750, and `/DW`
// 1000 — so a width read from `/W` is 12 points at size 12 and one read from `/DW` is 24.
func pagesSharingACIDFont(n int, directDescendant bool) []byte {
	desc := "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Shared " +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 8 0 R /DW 1000 /W 7 0 R >>"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		5: "<< /Type /Font /Subtype /Type0 /BaseFont /Shared /Encoding /Identity-H /DescendantFonts [6 0 R] >>",
		6: desc,
		7: "[65 [250 750]]",
		8: "<< /Type /FontDescriptor /FontName /Shared /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 /Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 >>",
	}
	if directDescendant {
		objs[5] = "<< /Type /Font /Subtype /Type0 /BaseFont /Shared /Encoding /Identity-H /DescendantFonts [" + desc + "] >>"
		delete(objs, 6)
	}
	var kids []string
	for i := 0; i < n; i++ {
		p, c := 100+2*i, 101+2*i
		kids = append(kids, fmt.Sprintf("%d 0 R", p))
		objs[p] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents %d 0 R >>", c)
		objs[c] = optStream("BT /F1 12 Tf 72 700 Td <00410042> Tj ET", "")
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n)
	return assembleFixture(objs)
}

// TestAFontsWidthTableIsReadOnceForThePagesThatShareIt — /pending 771. The font cache is the page walker's, so a
// loop over pages expanded a shared CID font's `/W` once a page: measured, 4.4 s over 800 pages for a
// 20,000-width table. The loop's one budget keeps the table between pages.
func TestAFontsWidthTableIsReadOnceForThePagesThatShareIt(t *testing.T) {
	const pages = 12
	read := func(direct bool) *formWalkBudget {
		t.Helper()
		ctx, err := inspectionRead(pagesSharingACIDFont(pages, direct))
		if err != nil {
			t.Fatal(err)
		}
		pgs := pdfread.Pages(ctx)
		if len(pgs) != pages {
			t.Fatalf("setup: %d pages, want %d", len(pgs), pages)
		}
		budget := newFormWalkBudget(len(pgs))
		for _, pg := range pgs {
			pr, err := readPageRuns(ctx, pg, budget)
			if err != nil {
				t.Fatal(err)
			}
			// STIMULUS, on every page: the width is the one `/W` gives, so the table each page used is the real one.
			if len(pr.runs) != 1 || pr.runs[0].widthSrc != widthFromW || math.Abs(pr.runs[0].width-12) > 1e-6 {
				t.Fatalf("page %d reads %+v, want one run 12 points wide measured from /W", pg.Nr, pr.runs)
			}
		}
		return budget
	}
	if b := read(false); b.wReads != 1 {
		t.Errorf("%d pages sharing one CID font expanded its /W %d times, want 1", pages, b.wReads)
	}
	// A direct descendant has no object number to be found again by: read where it is met, and still right.
	if b := read(true); b.wReads != pages || len(b.cidW) != 0 {
		t.Errorf("a direct descendant font was expanded %d times over %d pages and %d table(s) kept, want %d and 0", b.wReads, pages, len(b.cidW), pages)
	}
}

// TestAWidthTablePastTheCapIsReadAndNotKept — the kept tables are bounded (`maxHeldWidths`): one that would take
// the budget past it is still read, and is simply read again by the next page.
func TestAWidthTablePastTheCapIsReadAndNotKept(t *testing.T) {
	ctx, err := inspectionRead(pagesSharingACIDFont(2, false))
	if err != nil {
		t.Fatal(err)
	}
	budget := newFormWalkBudget(2)
	budget.cidWHeld = maxHeldWidths - 1 // room for one entry; the table has two
	for _, pg := range pdfread.Pages(ctx) {
		pr, err := readPageRuns(ctx, pg, budget)
		if err != nil || len(pr.runs) != 1 || pr.runs[0].widthSrc != widthFromW {
			t.Fatalf("page %d reads %+v (%v), want one run measured from /W", pg.Nr, pr.runs, err)
		}
	}
	if budget.wReads != 2 || len(budget.cidW) != 0 || budget.cidWHeld != maxHeldWidths-1 {
		t.Errorf("a table past the cap: %d reads, %d kept, %d entries held — want 2, 0, %d", budget.wReads, len(budget.cidW), budget.cidWHeld, maxHeldWidths-1)
	}
}
