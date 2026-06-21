package pdfops

import (
	"bytes"
	"math"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// sizedPDF builds an n-page PDF whose every page is exactly w×h points.
func sizedPDF(t *testing.T, n int, w, h float64) []byte {
	t.Helper()
	pages := make([]RasterPage, n)
	for i := range pages {
		pages[i] = rasterPage(t, w, h)
	}
	pdf, err := ImagesToPDF(pages)
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func dims(t *testing.T, pdf []byte) []types.Dim {
	t.Helper()
	d, err := api.PageDims(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func sizeEq(a types.Dim, w, h float64) bool {
	return math.Round(a.Width) == math.Round(w) && math.Round(a.Height) == math.Round(h)
}

// TestSplitPageGrid pins the page-count and per-tile dimensions for a 2×2 split,
// both as a pure crop (each tile a quarter of the page) and with resize (each
// tile scaled up to the original footprint), and checks the untouched
// neighbouring pages survive in place.
func TestSplitPageGrid(t *testing.T) {
	pdf := sizedPDF(t, 3, 120, 160) // pages 1..3, each 120×160

	// 2×2 crop of page 2: 3 → 3 + (4-1) = 6 pages; tiles are 60×80.
	out, err := SplitPage(pdf, 2, 2, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	d := dims(t, out)
	if len(d) != 6 {
		t.Fatalf("crop: page count = %d, want 6", len(d))
	}
	if !sizeEq(d[0], 120, 160) || !sizeEq(d[5], 120, 160) {
		t.Errorf("crop: neighbour pages resized to %v / %v, want 120×160", d[0], d[5])
	}
	for i := 1; i <= 4; i++ {
		if !sizeEq(d[i], 60, 80) {
			t.Errorf("crop: tile page %d = %.0f×%.0f, want 60×80", i+1, d[i].Width, d[i].Height)
		}
	}

	// 2×2 with resize: s = min(2,2) = 2, so each tile fills the 120×160 footprint.
	out, err = SplitPage(pdf, 2, 2, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	d = dims(t, out)
	if len(d) != 6 {
		t.Fatalf("resize: page count = %d, want 6", len(d))
	}
	for i := 1; i <= 4; i++ {
		if !sizeEq(d[i], 120, 160) {
			t.Errorf("resize: tile page %d = %.0f×%.0f, want 120×160", i+1, d[i].Width, d[i].Height)
		}
	}
}

// TestSplitPageEdges covers the splice guards: splitting the first page (no left
// segment) and the last page (no right segment), plus non-square grids.
func TestSplitPageEdges(t *testing.T) {
	pdf := sizedPDF(t, 3, 120, 160)

	// First page, 1×2 (two rows stacked): 3 → 4 pages; tiles 120×80, then the
	// two original pages follow.
	out, err := SplitPage(pdf, 1, 1, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	d := dims(t, out)
	if len(d) != 4 {
		t.Fatalf("first-page split: count = %d, want 4", len(d))
	}
	if !sizeEq(d[0], 120, 80) || !sizeEq(d[1], 120, 80) {
		t.Errorf("first-page tiles = %v / %v, want 120×80", d[0], d[1])
	}
	if !sizeEq(d[2], 120, 160) || !sizeEq(d[3], 120, 160) {
		t.Errorf("first-page trailing originals = %v / %v, want 120×160", d[2], d[3])
	}

	// Last page, 2×1 (two columns): 3 → 4 pages; the first two originals stay,
	// tiles 60×160 at the end.
	out, err = SplitPage(pdf, 3, 2, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	d = dims(t, out)
	if len(d) != 4 {
		t.Fatalf("last-page split: count = %d, want 4", len(d))
	}
	if !sizeEq(d[0], 120, 160) || !sizeEq(d[1], 120, 160) {
		t.Errorf("last-page leading originals = %v / %v, want 120×160", d[0], d[1])
	}
	if !sizeEq(d[2], 60, 160) || !sizeEq(d[3], 60, 160) {
		t.Errorf("last-page tiles = %v / %v, want 60×160", d[2], d[3])
	}
}

// TestSplitPageKeepsVectorContent proves the split is a re-crop, not a raster:
// text that lands in a tile is still live, extractable content afterwards.
func TestSplitPageKeepsVectorContent(t *testing.T) {
	pdf, err := testpdf.Text("topleftMARKER", "page two")
	if err != nil {
		t.Fatal(err)
	}
	// A4 page, text drawn at (100,700) — upper-left region. A 2×2 split keeps it
	// in the top-left tile, with the glyphs still in the content stream.
	out, err := SplitPage(pdf, 1, 2, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !contentContains(t, out, "", "topleftMARKER") {
		t.Error("split rasterized the page — text content is gone, want it preserved")
	}
}

// TestSplitPageRejectsBadInput covers the guards.
func TestSplitPageRejectsBadInput(t *testing.T) {
	pdf := sizedPDF(t, 2, 120, 160)
	if _, err := SplitPage(pdf, 1, 1, 1, false); err == nil {
		t.Error("1×1 split should error (no sub-pages)")
	}
	if _, err := SplitPage(pdf, 5, 2, 2, false); err == nil {
		t.Error("out-of-range page should error")
	}
}
