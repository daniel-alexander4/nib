package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// TestInsertPDF proves InsertPDF places the other document's pages immediately
// BEFORE the chosen page (before page 1 prepends), in order, leaving the host
// pages intact. Pages are built at distinct sizes so order — not just count — is
// provable by dimensions.
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

	// Insert before page 2: page1, other1, other2, page2, page3.
	mid, err := InsertPDF(base, other, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := api.PageDims(bytes.NewReader(mid), conf)
	exp := [][2]int{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 2}} // {kind, idx}: base0, other0, other1, base1, base2
	if len(got) != len(exp) {
		t.Fatalf("insert before 2: count = %d, want %d", len(got), len(exp))
	}
	for i, e := range exp {
		ref := baseDims[e[1]]
		if e[0] == 1 {
			ref = otherDims[e[1]]
		}
		if !sameDim(got[i], ref) {
			t.Errorf("insert before 2: page %d = %v, want %v", i+1, got[i], ref)
		}
	}

	// Insert before page 1 prepends: other1, other2, page1, page2, page3.
	pre, err := InsertPDF(base, other, 1)
	if err != nil {
		t.Fatal(err)
	}
	pd, _ := api.PageDims(bytes.NewReader(pre), conf)
	if len(pd) != 5 {
		t.Fatalf("prepend: count = %d, want 5", len(pd))
	}
	if !sameDim(pd[0], otherDims[0]) || !sameDim(pd[1], otherDims[1]) || !sameDim(pd[2], baseDims[0]) {
		t.Errorf("prepend: order wrong — got %v,%v,%v want other0,other1,base0", pd[0], pd[1], pd[2])
	}

	if _, err := InsertPDF(base, other, 0); err == nil {
		t.Error("insert at page 0 should error")
	}
	if _, err := InsertPDF(base, other, 4); err == nil {
		t.Error("insert past the last page should error")
	}
}

// TestDuplicatePage proves DuplicatePage emits the chosen page twice, in place,
// leaving the rest untouched — including the page-1 and last-page edges.
func TestDuplicatePage(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	base, err := ImagesToPDF([]RasterPage{
		rasterPage(t, 80, 110),
		rasterPage(t, 120, 90),
		rasterPage(t, 200, 150),
	})
	if err != nil {
		t.Fatal(err)
	}
	baseDims, _ := api.PageDims(bytes.NewReader(base), conf)

	// Duplicate page 2: page1, page2, page2, page3.
	dup, err := DuplicatePage(base, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := api.PageDims(bytes.NewReader(dup), conf)
	want := []int{0, 1, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("duplicate page 2: count = %d, want 4", len(got))
	}
	for i, b := range want {
		if !sameDim(got[i], baseDims[b]) {
			t.Errorf("duplicate page 2: page %d = %v, want base[%d] %v", i+1, got[i], b, baseDims[b])
		}
	}

	// Edge: duplicate the first page.
	first, _ := DuplicatePage(base, 1)
	fd, _ := api.PageDims(bytes.NewReader(first), conf)
	if len(fd) != 4 || !sameDim(fd[0], baseDims[0]) || !sameDim(fd[1], baseDims[0]) || !sameDim(fd[2], baseDims[1]) {
		t.Errorf("duplicate page 1: got %v, want base0,base0,base1,base2", fd)
	}

	// Edge: duplicate the last page.
	last, _ := DuplicatePage(base, 3)
	ld, _ := api.PageDims(bytes.NewReader(last), conf)
	if len(ld) != 4 || !sameDim(ld[2], baseDims[2]) || !sameDim(ld[3], baseDims[2]) {
		t.Errorf("duplicate last page: got %v, want …,base2,base2", ld)
	}

	if _, err := DuplicatePage(base, 0); err == nil {
		t.Error("duplicate page 0 should error")
	}
	if _, err := DuplicatePage(base, 4); err == nil {
		t.Error("duplicate past the last page should error")
	}
}
