package pdfops

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/testpdf"
)

// TestInsertPDF proves InsertPDF places the other document's pages immediately
// before or after the chosen page, in order, leaving the host pages intact. Pages
// are built at distinct sizes so order — not just count — is provable by dimensions.
func TestInsertPDF(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	base, err := ImagesToPDF([]RasterPage{
		rasterPage(t, 80, 110),
		rasterPage(t, 120, 90),
		rasterPage(t, 200, 150),
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := ImagesToPDF([]RasterPage{
		rasterPage(t, 40, 60),
		rasterPage(t, 50, 70),
	})
	if err != nil {
		t.Fatal(err)
	}
	baseDims, _ := api.PageDims(bytes.NewReader(base), conf)
	otherDims, _ := api.PageDims(bytes.NewReader(other), conf)

	// Each case lists the result as {kind, idx}: kind 0 is a base page, 1 an inserted one.
	b0, b1, b2, o0, o1 := [2]int{0, 0}, [2]int{0, 1}, [2]int{0, 2}, [2]int{1, 0}, [2]int{1, 1}
	for _, tc := range []struct {
		name   string
		page   int
		before bool
		want   [][2]int
	}{
		{"before page 1 prepends", 1, true, [][2]int{o0, o1, b0, b1, b2}},
		{"before page 2", 2, true, [][2]int{b0, o0, o1, b1, b2}},
		{"after page 1", 1, false, [][2]int{b0, o0, o1, b1, b2}},
		{"after page 2", 2, false, [][2]int{b0, b1, o0, o1, b2}},
		{"after the last page appends", 3, false, [][2]int{b0, b1, b2, o0, o1}},
	} {
		out, err := InsertPDF(base, other, tc.page, tc.before)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got, _ := api.PageDims(bytes.NewReader(out), conf)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: count = %d, want %d", tc.name, len(got), len(tc.want))
		}
		for i, e := range tc.want {
			ref := baseDims[e[1]]
			if e[0] == 1 {
				ref = otherDims[e[1]]
			}
			if !sameDim(got[i], ref) {
				t.Errorf("%s: page %d = %v, want %v", tc.name, i+1, got[i], ref)
			}
		}
	}

	// After the last page is Append, page for page.
	after, _ := InsertPDF(base, other, 3, false)
	appended, err := Append(base, other)
	if err != nil {
		t.Fatal(err)
	}
	ad, _ := api.PageDims(bytes.NewReader(after), conf)
	pd, _ := api.PageDims(bytes.NewReader(appended), conf)
	if len(ad) != len(pd) {
		t.Fatalf("after the last page has %d pages and Append %d", len(ad), len(pd))
	}
	for i := range pd {
		if !sameDim(ad[i], pd[i]) {
			t.Errorf("after the last page differs from Append at page %d: %v vs %v", i+1, ad[i], pd[i])
		}
	}

	for _, before := range []bool{true, false} {
		if _, err := InsertPDF(base, other, 0, before); err == nil {
			t.Errorf("insert at page 0 (before=%v) should error", before)
		}
		if _, err := InsertPDF(base, other, 4, before); err == nil {
			t.Errorf("insert past the last page (before=%v) should error", before)
		}
	}
}

// TestInsertBlankGoesOnTheSideAsked — before page 1 prepends, after the last page appends. A blank
// takes its neighbour's size, so "before page p" and "after page p" have the same widths and only the
// content can tell them apart: each base page draws its own word, and the blank is the page that draws
// nothing.
func TestInsertBlankGoesOnTheSideAsked(t *testing.T) {
	base, err := testpdf.Text("first", "second")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		page   int
		before bool
		want   []string // each page's text in the result; "" is the blank
	}{
		{"before page 1 prepends", 1, true, []string{"", "first", "second"}},
		{"after page 1", 1, false, []string{"first", "", "second"}},
		{"before page 2", 2, true, []string{"first", "", "second"}},
		{"after the last page appends", 2, false, []string{"first", "second", ""}},
	} {
		out, err := InsertBlank(base, tc.page, tc.before)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		if ctx.PageCount != len(tc.want) {
			t.Fatalf("%s: count = %d, want %d", tc.name, ctx.PageCount, len(tc.want))
		}
		for pg := 1; pg <= ctx.PageCount; pg++ {
			pr, err := readPageRuns(ctx, pg)
			if err != nil {
				t.Fatal(err)
			}
			var text string
			for _, r := range pr.runs {
				text += r.text
			}
			if strings.TrimSpace(text) != tc.want[pg-1] {
				t.Errorf("%s: page %d reads %q, want %q", tc.name, pg, text, tc.want[pg-1])
			}
		}
	}
}
