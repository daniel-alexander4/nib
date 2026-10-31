package pdfops

import (
	"errors"
	"strings"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 907` — a mark on every page. The pages a mark lands on and WHERE it lands are read from each
// page's content stream; the assertions would fail on a build that stamped every page, none, or the wrong corner.

// marked reports whether a page's content draws a stamp: the stamp is a form drawn under an /Artifact
// Watermark, which none of the test pages carry of their own.
func marked(t *testing.T, pdf []byte, page int) string {
	t.Helper()
	c := string(pageContentOf(t, pdf, page))
	if !strings.Contains(c, "/Watermark") {
		return ""
	}
	i := strings.Index(c, "BDC q ")
	if i < 0 {
		t.Fatalf("page %d carries a watermark artifact but no stamp placement: %s", page, c)
	}
	return c[i:]
}

func TestStampPagesImageLandsOnTheSelectedPagesInTheNamedCorner(t *testing.T) {
	doc, err := testpdf.Text("one", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampPages(doc, PageStamp{Kind: StampKindImage, Image: pngBytes(t, 100, 50), Position: "br", Scale: 0.25, Pages: []string{"2-3"}})
	if err != nil {
		t.Fatal(err)
	}
	if m := marked(t, out, 1); m != "" {
		t.Errorf("page 1 was not selected and carries a stamp: %s", m)
	}
	// The test pages are 595 wide: 25% is 148.75 pt, 36 pt in from the right edge puts its left at 595-36-148.75.
	for _, p := range []int{2, 3} {
		if m := marked(t, out, p); !strings.Contains(m, "410.25000 36.00000 cm") {
			t.Errorf("page %d: stamp placement = %q, want the bottom-right corner at 410.25, 36", p, m)
		}
	}
}

func TestStampPagesTextIsPlacedAndEveryPageIsTheDefault(t *testing.T) {
	doc, err := testpdf.Text("one", "two")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampPages(doc, PageStamp{Kind: StampKindText, Text: "Draft 100%", Position: "bl"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []int{1, 2} {
		if m := marked(t, out, p); !strings.Contains(m, "36.00000 36.00000 cm") {
			t.Errorf("page %d: stamp placement = %q, want the bottom-left corner at 36, 36", p, m)
		}
	}
}

func TestStampPagesWatermarkHonoursThePageSelection(t *testing.T) {
	doc, err := testpdf.Text("one", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampPages(doc, PageStamp{Kind: StampKindWatermark, Text: "COPY", Pages: []string{"1", "3"}})
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[int]bool{1: true, 2: false, 3: true} {
		if got := marked(t, out, p) != ""; got != want {
			t.Errorf("page %d stamped = %v, want %v", p, got, want)
		}
	}
}

// fillAlphas lists every /ca (fill opacity) the document's graphics states carry.
func fillAlphas(t *testing.T, pdf []byte) []float64 {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	var out []float64
	for _, e := range ctx.Table {
		if e == nil || e.Free {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		switch v := d["ca"].(type) {
		case types.Float:
			out = append(out, float64(v))
		case types.Integer:
			out = append(out, float64(v))
		}
	}
	return out
}

func TestStampPagesOpacityIsDrawn(t *testing.T) {
	doc, err := testpdf.Text("one")
	if err != nil {
		t.Fatal(err)
	}
	for name, st := range map[string]PageStamp{
		"image": {Kind: StampKindImage, Image: pngBytes(t, 40, 20), Opacity: 0.3},
		"text":  {Kind: StampKindText, Text: "Draft", Opacity: 0.3},
	} {
		out, err := StampPages(doc, st)
		if err != nil {
			t.Fatal(err)
		}
		got := fillAlphas(t, out)
		found := false
		for _, a := range got {
			found = found || (a > 0.29 && a < 0.31)
		}
		if !found {
			t.Errorf("%s: fill opacities in the stamped document = %v, want one at 0.3", name, got)
		}
	}
}

func TestStampPagesSelectionHonoursANegatedTerm(t *testing.T) {
	doc, err := testpdf.Text("one", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampPages(doc, PageStamp{Kind: StampKindImage, Image: pngBytes(t, 40, 20), Pages: []string{"1-3", "!2"}})
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[int]bool{1: true, 2: false, 3: true} {
		if got := marked(t, out, p) != ""; got != want {
			t.Errorf("page %d stamped = %v, want %v", p, got, want)
		}
	}
}

func TestStampPagesRefusesWhatItCannotDraw(t *testing.T) {
	doc, err := testpdf.Text("one", "two")
	if err != nil {
		t.Fatal(err)
	}
	var perr *PageRangeError
	if _, err := StampPages(doc, PageStamp{Kind: StampKindText, Text: "x", Pages: []string{"7"}}); !errors.As(err, &perr) {
		t.Errorf("a page the document does not have = %v, want a PageRangeError", err)
	}
	for name, st := range map[string]PageStamp{
		"text":      {Kind: StampKindText, Text: "  "},
		"watermark": {Kind: StampKindWatermark},
		"image":     {Kind: StampKindImage},
	} {
		if _, err := StampPages(doc, st); !errors.Is(err, ErrStampNeedsContent) {
			t.Errorf("an empty %s = %v, want ErrStampNeedsContent", name, err)
		}
	}
	if _, err := StampPages(doc, PageStamp{Kind: "sticker", Text: "x"}); err == nil {
		t.Error("an unknown kind was stamped")
	}
}
