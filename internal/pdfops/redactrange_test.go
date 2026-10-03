package pdfops

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"

	"nib/internal/testpdf"
)

// TestRedactPagesRefusesAPageTheDocumentDoesNotHave: a raster keyed outside 1..n used to be
// skipped by the 1..n loop, so RedactPages succeeded and the page it meant kept its vector text
// (/pending 819). Measured before the fix on the one-page form fixture: keys 2, 0 and -1 each
// returned err=nil with "fullName" still present. Every such key — and an empty raster — must now
// refuse with ErrRedactPageOutOfRange and return no bytes.
func TestRedactPagesRefusesAPageTheDocumentDoesNotHave(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 100, 100))); err != nil {
		t.Fatal(err)
	}
	page := RasterPage{Image: img.Bytes(), W: 612, H: 792}

	for _, k := range []int{2, 0, -1} {
		// Alone, and beside a valid key: the valid one must not carry the bad one through.
		for _, raster := range []map[int]RasterPage{{k: page}, {1: page, k: page}} {
			out, err := RedactPages(form, raster)
			if !errors.Is(err, ErrRedactPageOutOfRange) {
				t.Fatalf("key %d (raster of %d): err = %v, want ErrRedactPageOutOfRange", k, len(raster), err)
			}
			var pe *RedactPageError
			if !errors.As(err, &pe) || pe.Page != k || pe.Pages != 1 {
				t.Errorf("key %d: refusal = %#v, want Page %d of 1", k, pe, k)
			}
			if out != nil {
				t.Errorf("key %d: refusal returned %d bytes, want none", k, len(out))
			}
		}
	}
	if out, err := RedactPages(form, map[int]RasterPage{}); !errors.Is(err, ErrRedactPageOutOfRange) || out != nil {
		t.Errorf("empty raster: (%d bytes, %v), want a refusal and no bytes", len(out), err)
	}

	// The in-range key still redacts: the check refuses only what names no page.
	out, err := RedactPages(form, map[int]RasterPage{1: page})
	if err != nil {
		t.Fatalf("in-range key: %v", err)
	}
	if j, _ := ExportFormJSON(out); bytes.Contains(j, []byte("fullName")) {
		t.Error("in-range redaction kept the page's field")
	}
}
