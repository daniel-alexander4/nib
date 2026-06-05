package pdfops

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// TestStampImagesVisual writes a PDF with a red square stamped at a known
// rectangle so it can be rasterised (pdftoppm) and the placement eyeballed.
// Set NIB_STAMP_OUT to a path to emit the PDF; otherwise it just checks the
// pipeline returns a valid, same-page-count document.
func TestStampImagesVisual(t *testing.T) {
	// White letter page background.
	bg := image.NewRGBA(image.Rect(0, 0, 612, 792))
	for i := range bg.Pix {
		bg.Pix[i] = 0xff
	}
	var bgBuf bytes.Buffer
	if err := png.Encode(&bgBuf, bg); err != nil {
		t.Fatal(err)
	}
	base, err := ImagesToPDF([]RasterPage{{Image: bgBuf.Bytes(), W: 612, H: 792}})
	if err != nil {
		t.Fatal(err)
	}

	// A solid red 100x40 square stamp.
	sq := image.NewRGBA(image.Rect(0, 0, 100, 40))
	for x := 0; x < 100; x++ {
		for y := 0; y < 40; y++ {
			sq.Set(x, y, color.RGBA{220, 0, 0, 255})
		}
	}
	var sqBuf bytes.Buffer
	if err := png.Encode(&sqBuf, sq); err != nil {
		t.Fatal(err)
	}

	// Rectangle in PDF points (bottom-left origin): a 200x80 box near
	// bottom-left, so x0=100,y0=100,x1=300,y1=180.
	out, err := StampImages(base, []Stamp{{Page: 1, Rect: [4]float64{100, 100, 300, 180}, PNG: sqBuf.Bytes()}})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(out); n != 1 {
		t.Fatalf("page count = %d, want 1", n)
	}
	if p := os.Getenv("NIB_STAMP_OUT"); p != "" {
		if err := os.WriteFile(p, out, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", p)
	}
}
