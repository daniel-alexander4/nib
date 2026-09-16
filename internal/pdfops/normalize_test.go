package pdfops

import (
	"bytes"
	"math"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestTheModalPageSizeIsTheSameOnEveryRun — `/pending 503`. 500×800 and 400×1000 tie on count and area;
// ranked by those alone the target followed Go's randomised map order.
func TestTheModalPageSizeIsTheSameOnEveryRun(t *testing.T) {
	dims := []types.Dim{{Width: 500, Height: 800}, {Width: 400, Height: 1000}}
	first := modalPageDim(dims)
	for i := 0; i < 200; i++ {
		if got := modalPageDim(dims); got != first {
			t.Fatalf("call %d chose %v and the first call chose %v — the same document normalises to a different size per run", i+2, got, first)
		}
	}
	if got := modalPageDim([]types.Dim{dims[1], dims[0]}); got != first {
		t.Errorf("the same sizes in the other order chose %v, not %v", got, first)
	}
}

func appendAll(t *testing.T, pdfs ...[]byte) []byte {
	t.Helper()
	out := pdfs[0]
	for _, p := range pdfs[1:] {
		var err error
		if out, err = Append(out, p); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func dimsClose(a, b types.Dim) bool {
	return math.Abs(a.Width-b.Width) < 1 && math.Abs(a.Height-b.Height) < 1
}

// TestNormalizePageSizes: a document of three 612×792 pages plus one 420×700 page
// is resized so every page becomes the modal (612×792) size — the odd page scaled
// to fit, not clipped.
func TestNormalizePageSizes(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	modal := fourQuadrantPDF(t, 612, 792)
	odd := fourQuadrantPDF(t, 420, 700)
	mixed := appendAll(t, modal, modal, modal, odd) // 3 modal + 1 odd

	out, err := NormalizePageSizes(mixed)
	if err != nil {
		t.Fatal(err)
	}
	dims, err := api.PageDims(bytes.NewReader(out), conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != 4 {
		t.Fatalf("normalized to %d pages, want 4", len(dims))
	}
	want := types.Dim{Width: 612, Height: 792}
	for i, d := range dims {
		if !dimsClose(d, want) {
			t.Errorf("page %d is %.1f×%.1f, want uniform %g×%g", i+1, d.Width, d.Height, want.Width, want.Height)
		}
	}
}

// TestNormalizePageSizesUniform: an already-uniform document stays uniform and
// errors nowhere (the no-op case).
func TestNormalizePageSizesUniform(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	uni := appendAll(t, fourQuadrantPDF(t, 612, 792), fourQuadrantPDF(t, 612, 792))
	out, err := NormalizePageSizes(uni)
	if err != nil {
		t.Fatal(err)
	}
	dims, _ := api.PageDims(bytes.NewReader(out), conf)
	for i, d := range dims {
		if !dimsClose(d, types.Dim{Width: 612, Height: 792}) {
			t.Errorf("page %d is %.1f×%.1f, want 612×792", i+1, d.Width, d.Height)
		}
	}
}
