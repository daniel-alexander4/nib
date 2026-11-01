package pdfops

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// `/pending 929` — a contents page. The text is read back through DocumentText (the page map), and the numbers it prints
// are the assertion: they are the NEW physical numbers, so a layout that forgot the shift is wrong on every line.

func contentsText(t *testing.T, pdf []byte, page int) string {
	t.Helper()
	pts, err := DocumentText(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, r := range pts[page-1].Runs {
		b.WriteString(r.Text)
		b.WriteString("|")
	}
	return b.String()
}

func bookmarked(t *testing.T, pages int, items []OutlineItem) []byte {
	t.Helper()
	labels := make([]string, pages)
	for i := range labels {
		labels[i] = "body"
	}
	doc, err := testpdf.Text(labels...)
	if err != nil {
		t.Fatal(err)
	}
	out, err := SetOutline(doc, items)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestInsertContentsListsTheBookmarksWithTheNewPageNumbers(t *testing.T) {
	doc := bookmarked(t, 5, []OutlineItem{
		{Title: "Alpha", Page: 1, Level: 0}, {Title: "Beta", Page: 3, Level: 0}, {Title: "Beta detail", Page: 4, Level: 1}, {Title: "Gamma", Page: 5, Level: 0},
	})
	out, rep, err := InsertContents(doc, ContentsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pages != 1 || rep.Entries != 4 || rep.Source != "bookmarks" {
		t.Fatalf("report %+v", rep)
	}
	if n, _ := PageCount(out); n != 6 {
		t.Fatalf("the document has %d pages, want 6", n)
	}
	got := contentsText(t, out, 1)
	for _, want := range []string{"Contents|", "Alpha|", "Beta|", "Beta detail|", "Gamma|", "2|", "4|", "5|", "6|"} {
		if !strings.Contains(got, want) {
			t.Errorf("the contents page lacks %q: %s", want, got)
		}
	}
	// The old page numbers must not be printed: Alpha was on page 1 and is now on page 2.
	if strings.Contains(got, "|1|") || strings.HasSuffix(got, "|1|") {
		t.Errorf("an entry shows its old page number: %s", got)
	}
	// The bookmarks still reach their pages (a destination names a page object).
	back, err := Outline(out)
	if err != nil || len(back) != 4 || back[1].Page != 4 {
		t.Errorf("the outline became %+v (%v); Beta was page 3 and is page 4 now", back, err)
	}
}

func TestInsertContentsAfterAPageLeavesTheEarlierPagesWhereTheyAre(t *testing.T) {
	doc := bookmarked(t, 4, []OutlineItem{{Title: "One", Page: 1, Level: 0}, {Title: "Three", Page: 3, Level: 0}})
	out, _, err := InsertContents(doc, ContentsOptions{After: 1, Title: "In this report"})
	if err != nil {
		t.Fatal(err)
	}
	got := contentsText(t, out, 2)
	if !strings.Contains(got, "In this report|") || !strings.Contains(got, "One|") || !strings.Contains(got, "Three|") {
		t.Fatalf("page 2 is not the contents page: %s", got)
	}
	// One stays on page 1, before the insertion; Three moves from 3 to 4.
	if !strings.Contains(got, "|1|") || !strings.Contains(got, "|4|") {
		t.Errorf("the numbers are wrong for a page inserted after page 1: %s", got)
	}
}

func TestInsertContentsSpillsOntoASecondPageAndCountsIt(t *testing.T) {
	var items []OutlineItem
	for i := 1; i <= 80; i++ {
		items = append(items, OutlineItem{Title: "Section " + strings.Repeat("x", i%7+1), Page: 1 + i%5, Level: 0})
	}
	// Titles must be unique for SetOutline, so give the document no outline and pass the list.
	doc := bookmarked(t, 6, []OutlineItem{{Title: "only", Page: 1}})
	out, rep, err := InsertContents(doc, ContentsOptions{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pages < 2 {
		t.Fatalf("80 entries fit on %d page(s)", rep.Pages)
	}
	if n, _ := PageCount(out); n != 6+rep.Pages {
		t.Errorf("the document has %d pages, want %d", n, 6+rep.Pages)
	}
	// Entry pages are 2..6 and every one is shifted by the block: nothing printed may be below 1+Pages.
	last := contentsText(t, out, rep.Pages)
	if strings.Contains(last, "|2|") && !strings.Contains(last, "|"+strconv.Itoa(2+rep.Pages)+"|") {
		t.Errorf("the numbers were not shifted by the block of %d: %s", rep.Pages, last)
	}
}

func TestInsertContentsRefusesWhatItCannotDo(t *testing.T) {
	plain, err := testpdf.Text("x", "y")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := InsertContents(plain, ContentsOptions{}); !errors.Is(err, ErrNoContentsEntries) {
		t.Errorf("a document with nothing to list = %v, want ErrNoContentsEntries", err)
	}
	doc := bookmarked(t, 2, []OutlineItem{{Title: "a", Page: 1}})
	var perr *PageRangeError
	if _, _, err := InsertContents(doc, ContentsOptions{After: 9}); !errors.As(err, &perr) {
		t.Errorf("a position past the end = %v, want a PageRangeError", err)
	}
	if _, _, err := InsertContents(doc, ContentsOptions{Items: []OutlineItem{{Title: "z", Page: 7}}}); !errors.As(err, &perr) {
		t.Errorf("an entry naming a page the document lacks = %v, want a PageRangeError", err)
	}
	if _, _, err := InsertContents(doc, ContentsOptions{Items: []OutlineItem{{Title: "deep", Page: 1, Level: 2}}, Depth: 1}); !errors.Is(err, ErrNoContentsEntries) {
		t.Errorf("every entry below the depth = %v, want ErrNoContentsEntries", err)
	}
}

func TestInsertContentsCarriesPageLabelsPastTheNewPage(t *testing.T) {
	doc := bookmarked(t, 4, []OutlineItem{{Title: "a", Page: 1}})
	labelled, err := SetPageLabels(doc, []PageLabelRange{{Start: 1, Style: "roman-lower", First: 1}, {Start: 3, Style: "decimal", First: 1}})
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := InsertContents(labelled, ContentsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.Validated(out, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	nums, err := ctx.DereferenceArray(derefDict(ctx.XRefTable, root["PageLabels"])["Nums"])
	if err != nil {
		t.Fatalf("the output has no page labels: %v", err)
	}
	// Page 1 (the contents) unlabelled, pages 2-3 roman, pages 4-5 decimal from 1: three ranges at 1, 2 and 4.
	var starts []int
	for _, r := range readPageLabelRanges(ctx.XRefTable, nums) {
		starts = append(starts, r.start+1)
	}
	if len(starts) != 3 || starts[0] != 1 || starts[1] != 2 || starts[2] != 4 {
		t.Errorf("label ranges start at %v, want [1 2 4]", starts)
	}
}
