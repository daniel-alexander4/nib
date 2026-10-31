package cli

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// TestMergeTakesAnImageAsThePageTheOpenerMakesOfIt — /pending 909. `nib merge` reads a PNG through
// `pdfops.ImageToDocument`, the door the window's opener uses, so a 192×96 picture is a 144×72 pt
// page (96 dpi) in the position it was named, beside a PDF page of the producer's own size. A
// non-image, non-PDF input is still refused.
func TestMergeTakesAnImageAsThePageTheOpenerMakesOfIt(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 192, 96))); err != nil {
		t.Fatal(err)
	}
	pic := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(pic, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := writePDF(t, dir, "a.pdf", "x")
	out := filepath.Join(dir, "out.pdf")

	if code := cmdMerge([]string{pic, doc, "-o", out}); code != 0 {
		t.Fatalf("merge exit = %d, want 0", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	dims, err := api.PageDims(context.Background(), bytes.NewReader(b), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != 2 || math.Round(dims[0].Width) != 144 || math.Round(dims[0].Height) != 72 {
		t.Fatalf("pages = %+v, want the picture first at 144×72 then the PDF's page", dims)
	}

	junk := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(junk, []byte("plain text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdMerge([]string{pic, junk, "-o", filepath.Join(dir, "no.pdf")}); code == 0 {
		t.Error("a text file was merged; only PDFs and images are inputs")
	}
}
