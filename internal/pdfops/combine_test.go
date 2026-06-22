package pdfops

import (
	"bytes"
	"math"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// TestCombineMergesInOrder proves Combine concatenates documents in the given
// order, each keeping its own page size — built at distinct widths so order is
// provable, not just the count.
func TestCombineMergesInOrder(t *testing.T) {
	mk := func(w, h float64) []byte {
		pdf, err := ImagesToPDF([]RasterPage{rasterPage(t, w, h)})
		if err != nil {
			t.Fatal(err)
		}
		return pdf
	}
	out, err := Combine([][]byte{mk(80, 110), mk(120, 90), mk(200, 150)})
	if err != nil {
		t.Fatal(err)
	}
	dims, err := api.PageDims(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{80, 120, 200}
	if len(dims) != len(want) {
		t.Fatalf("combined page count = %d, want %d", len(dims), len(want))
	}
	for i, wv := range want {
		if math.Round(dims[i].Width) != wv {
			t.Errorf("page %d width = %.1f, want %.0f (order not preserved)", i+1, dims[i].Width, wv)
		}
	}
}

// TestCombineSingleAndEmpty proves the edge cases: one document is returned
// unchanged (MergeRaw needs two), and zero documents is an error.
func TestCombineSingleAndEmpty(t *testing.T) {
	a, err := ImagesToPDF([]RasterPage{rasterPage(t, 80, 110)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Combine([][]byte{a})
	if err != nil || !bytes.Equal(got, a) {
		t.Errorf("Combine of one doc should return it unchanged (err %v)", err)
	}
	if _, err := Combine(nil); err == nil {
		t.Error("Combine of zero docs should error")
	}
}
