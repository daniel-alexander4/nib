package pdfops

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{0, 128, 255, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// rasterPage builds a RasterPage whose image is rendered at 2× the target point
// size, mirroring how the client rasterizes (scale 2) before upload.
func rasterPage(t *testing.T, wPt, hPt float64) RasterPage {
	t.Helper()
	return RasterPage{Image: pngBytes(t, int(wPt*2), int(hPt*2)), W: wPt, H: hPt}
}

func TestImagesToPDF(t *testing.T) {
	pdf, err := ImagesToPDF([]RasterPage{rasterPage(t, 100, 140), rasterPage(t, 100, 140)})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Errorf("ImagesToPDF did not produce a PDF: %.10q", pdf)
	}
}

// TestImagesToPDFPageSizes pins the rasterize-2× page-size fix: a page rasterized
// at 2× (image is double its point size) must come out at its TRUE point size,
// not the doubled pixel size, and a mix of sizes in one call is preserved per
// page (the merge path).
func TestImagesToPDFPageSizes(t *testing.T) {
	pages := []RasterPage{
		{Image: pngBytes(t, 1224, 1584), W: 612, H: 792}, // Letter, rasterized 2×
		{Image: pngBytes(t, 1190, 1684), W: 595, H: 842}, // A4, rasterized 2×
	}
	pdf, err := ImagesToPDF(pages)
	if err != nil {
		t.Fatal(err)
	}
	dims, err := api.PageDims(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != len(pages) {
		t.Fatalf("page count = %d, want %d", len(dims), len(pages))
	}
	for i, p := range pages {
		if math.Round(dims[i].Width) != math.Round(p.W) || math.Round(dims[i].Height) != math.Round(p.H) {
			t.Errorf("page %d size = %.1f×%.1f pt, want %.0f×%.0f (raster resolution must not change the page size)",
				i+1, dims[i].Width, dims[i].Height, p.W, p.H)
		}
	}
}

func TestExportFormJSON(t *testing.T) {
	pdf, _ := testpdf.Form()
	data, err := ExportFormJSON(pdf)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture defines a "fullName" text field; it must appear in the export.
	if !bytes.Contains(data, []byte("fullName")) {
		t.Errorf("form export missing field name; got: %s", data)
	}
}

func threePagePDF(t *testing.T) []byte {
	t.Helper()
	pdf, err := ImagesToPDF([]RasterPage{rasterPage(t, 80, 110), rasterPage(t, 80, 110), rasterPage(t, 80, 110)})
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func TestPageOps(t *testing.T) {
	pdf := threePagePDF(t)
	if n, _ := PageCount(pdf); n != 3 {
		t.Fatalf("base page count = %d, want 3", n)
	}

	removed, err := RemovePages(pdf, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(removed); n != 2 {
		t.Errorf("after remove: count = %d, want 2", n)
	}

	reordered, err := Collect(pdf, []string{"3", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(reordered); n != 2 {
		t.Errorf("after reorder/collect: count = %d, want 2", n)
	}

	extracted, err := Collect(pdf, []string{"2-3"})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(extracted); n != 2 {
		t.Errorf("after extract subset: count = %d, want 2", n)
	}

	inserted, err := InsertBlank(pdf, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(inserted); n != 4 {
		t.Errorf("after insert blank: count = %d, want 4", n)
	}

	rotated, err := Rotate(pdf, []string{"1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(rotated); n != 3 {
		t.Errorf("after rotate: count = %d, want 3", n)
	}

	merged, err := Append(pdf, threePagePDF(t))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(merged); n != 6 {
		t.Errorf("after append: count = %d, want 6", n)
	}
}

func TestRedactRemovesContent(t *testing.T) {
	// The form fixture has a "fullName" field on page 1. Redacting that page must
	// replace it with a flat image, so the field is GONE, not merely covered.
	original, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if before, _ := ExportFormJSON(original); !bytes.Contains(before, []byte("fullName")) {
		t.Fatal("fixture should contain the fullName field before redaction")
	}

	redacted, err := RedactPages(original, map[int]RasterPage{1: rasterPage(t, 612, 792)})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(redacted); n != 1 {
		t.Errorf("redacted page count = %d, want 1", n)
	}
	after, _ := ExportFormJSON(redacted)
	if bytes.Contains(after, []byte("fullName")) {
		t.Error("redacted page still exposes the form field — content not truly removed")
	}
}

func TestRedactKeepsOtherPagesVector(t *testing.T) {
	original := threePagePDF(t)
	redacted, err := RedactPages(original, map[int]RasterPage{2: rasterPage(t, 80, 110)})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(redacted); n != 3 {
		t.Errorf("redacted doc page count = %d, want 3 (only page 2 replaced)", n)
	}
}

func TestStampWatermark(t *testing.T) {
	pdf := threePagePDF(t)
	faint := WatermarkStyle{Color: "#8a8a8a", Opacity: 0.1, Scale: 0.9, Angle: 45}
	out, err := StampWatermark(pdf, "DRAFT", faint)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(out); n != 3 {
		t.Errorf("watermarked page count = %d, want 3 (all pages kept)", n)
	}
	if len(out) <= len(pdf) {
		t.Error("watermarked PDF is not larger than the original (nothing added?)")
	}
	// A bold on-top style must also produce a valid all-pages doc.
	bold := WatermarkStyle{Color: "#cc0000", Opacity: 0.65, Scale: 0.9, Angle: 30}
	if v, err := StampWatermark(pdf, "VOID", bold); err != nil {
		t.Fatalf("bold watermark: %v", err)
	} else if n, _ := PageCount(v); n != 3 {
		t.Errorf("on-top watermarked page count = %d, want 3", n)
	}
	// Empty text returns the input unchanged.
	same, err := StampWatermark(pdf, "  ", faint)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(same, pdf) {
		t.Error("stamping with empty text should return the input unchanged")
	}
}

func TestStampPageNumbers(t *testing.T) {
	pdf := threePagePDF(t)
	// Bates-style: prefix + zero-pad + a corner position.
	out, err := StampPageNumbers(pdf, PageNumberStyle{Position: "br", Prefix: "ABC", Start: 1, Pad: 6})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("StampPageNumbers did not produce a PDF: %.10q", out)
	}
	if n, _ := PageCount(out); n != 3 {
		t.Errorf("numbered page count = %d, want 3 (all pages kept)", n)
	}
	if len(out) <= len(pdf) {
		t.Error("numbered PDF is not larger than the original (nothing stamped?)")
	}
	// Plain "n of N" at the default centre position must also produce a valid doc.
	if v, err := StampPageNumbers(pdf, PageNumberStyle{Start: 1, OfTotal: true}); err != nil {
		t.Fatalf("of-total numbering: %v", err)
	} else if n, _ := PageCount(v); n != 3 {
		t.Errorf("of-total page count = %d, want 3", n)
	}
	// A bad position falls back to bc and a '%' in the prefix is stripped (it would
	// otherwise be read as a pdfcpu placeholder token) — neither should error.
	if _, err := StampPageNumbers(pdf, PageNumberStyle{Position: "x, url:evil", Prefix: "50% "}); err != nil {
		t.Errorf("position fallback / prefix sanitize errored: %v", err)
	}
}

func TestWatermarkStyleSanitize(t *testing.T) {
	// A colour with description-breaking characters must be rejected (so it can't
	// inject extra pdfcpu watermark keys), and out-of-range numbers clamped.
	got := WatermarkStyle{Color: "#000, url:evil", Opacity: 9, Scale: 0, Angle: 400}.sanitize()
	if got.Color != "#8a8a8a" {
		t.Errorf("bad colour = %q, want fallback #8a8a8a", got.Color)
	}
	if got.Opacity != 1 || got.Scale != 0.1 || got.Angle != 90 {
		t.Errorf("clamp = {op:%v sc:%v ang:%d}, want {1 0.1 90}", got.Opacity, got.Scale, got.Angle)
	}
	// A valid style passes through unchanged.
	ok := WatermarkStyle{Color: "#A1b2C3", Opacity: 0.3, Scale: 0.5, Angle: -45}.sanitize()
	if ok.Color != "#A1b2C3" || ok.Opacity != 0.3 || ok.Scale != 0.5 || ok.Angle != -45 {
		t.Errorf("valid style mutated: %+v", ok)
	}
}

func TestStampFields(t *testing.T) {
	// One-page PDF from a blank image, then stamp a text field and a check.
	pdf, err := ImagesToPDF([]RasterPage{{Image: pngBytes(t, 400, 560), W: 400, H: 560}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampFields(pdf, []Field{
		{Page: 1, Rect: [4]float64{72, 700, 300, 716}, Text: "Jane Doe"},
		{Page: 1, Rect: [4]float64{72, 650, 86, 664}, Text: "X"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("StampFields did not produce a PDF: %.10q", out)
	}
	if len(out) <= len(pdf) {
		t.Error("stamped PDF is not larger than the original (nothing added?)")
	}
	// Empty fields are skipped, returning the input unchanged.
	same, err := StampFields(pdf, []Field{{Page: 1, Text: "   "}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(same, pdf) {
		t.Error("stamping only-empty fields should return the input unchanged")
	}
}

// TestStampFieldsStyle covers the cover-and-replace overrides: an explicit font,
// size and colour must produce a valid PDF, and the page must stay vector (a
// stamped text watermark, not a rasterized page).
func TestStampFieldsStyle(t *testing.T) {
	pdf, err := testpdf.Form() // a vector page with a real text layer
	if err != nil {
		t.Fatal(err)
	}
	out, err := StampFields(pdf, []Field{
		{Page: 1, Rect: [4]float64{72, 700, 300, 716}, Text: "Replacement", Font: "Times-BoldItalic", Size: 13, Color: "#cc0000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) || len(out) <= len(pdf) {
		t.Fatalf("styled StampFields did not add content to a PDF")
	}
	// The page is still vector: its original text layer survives (the edit is an
	// overlay, not a whole-page raster as redaction would be).
	if before, _ := ExportFormJSON(pdf); bytes.Contains(before, []byte("fullName")) {
		if after, _ := ExportFormJSON(out); !bytes.Contains(after, []byte("fullName")) {
			t.Error("styled stamp rasterized the page — the form field vanished")
		}
	}
}

func TestCoreFont(t *testing.T) {
	for _, name := range []string{"Times-Roman", "Courier-Bold", "Helvetica-Oblique"} {
		if got := coreFont(name); got != name {
			t.Errorf("coreFont(%q) = %q, want passthrough", name, got)
		}
	}
	// An unlisted or injection-laden name falls back to Helvetica.
	for _, bad := range []string{"Comic Sans", "Helvetica, opacity:0", ""} {
		if got := coreFont(bad); got != "Helvetica" {
			t.Errorf("coreFont(%q) = %q, want Helvetica fallback", bad, got)
		}
	}
}
