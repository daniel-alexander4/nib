package pdfops

import (
	"archive/zip"
	"bytes"
	"image/png"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestExtractImagesZip pins the embedded-image extraction: two pages carrying
// differently sized images yield two distinct image objects, bundled as a ZIP of
// valid image files. (pdfcpu re-encodes RGB rasters to PNG, so the entries are
// .png here.)
func TestExtractImagesZip(t *testing.T) {
	pdf, err := ImagesToPDF([]RasterPage{rasterPage(t, 100, 140), rasterPage(t, 90, 120)})
	if err != nil {
		t.Fatal(err)
	}
	data, count, err := ExtractImagesZip(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("extracted %d images, want 2", count)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != count {
		t.Fatalf("zip has %d entries, want %d", len(zr.File), count)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".png") {
			t.Errorf("entry %q does not carry pdfcpu's reported extension", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := png.Decode(rc); err != nil {
			t.Errorf("entry %q is not a valid image: %v", f.Name, err)
		}
		rc.Close()
	}
}

// TestExtractImagesPerPage drives the resilient fallback path directly: the
// per-page loop must extract every image and dedup across pages exactly like the
// whole-doc pass (it's what runs when one bad image aborts the fast path).
func TestExtractImagesPerPage(t *testing.T) {
	pdf, err := ImagesToPDF([]RasterPage{rasterPage(t, 100, 140), rasterPage(t, 90, 120)})
	if err != nil {
		t.Fatal(err)
	}
	data, count, err := extractImages(pdf, true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("per-page extraction got %d images, want 2", count)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("zip has %d entries, want 2", len(zr.File))
	}
}

// TestExtractImagesZipNone: a document with no embedded raster images yields no
// archive and a zero count (not an empty zip), so the handler can say "none".
func TestExtractImagesZipNone(t *testing.T) {
	pdf, _ := testpdf.Form()
	data, count, err := ExtractImagesZip(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || data != nil {
		t.Fatalf("form with no images: count=%d, %d bytes; want 0 and nil", count, len(data))
	}
}
