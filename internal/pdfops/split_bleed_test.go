package pdfops

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

// fourQuadrantPDF builds a one-page w×h-point PDF whose page is a single image
// divided into four solid-colour quadrants: top-left red, top-right green,
// bottom-left blue, bottom-right yellow.
func fourQuadrantPDF(t *testing.T, w, h float64) []byte {
	t.Helper()
	pw, ph := int(w*2), int(h*2)
	img := image.NewRGBA(image.Rect(0, 0, pw, ph))
	quad := [4]color.RGBA{{220, 30, 30, 255}, {30, 200, 30, 255}, {30, 60, 220, 255}, {230, 200, 20, 255}}
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			q := 0
			if x >= pw/2 {
				q++
			}
			if y >= ph/2 {
				q += 2
			}
			img.Set(x, y, quad[q])
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	pdf, err := ImagesToPDF([]RasterPage{{Image: buf.Bytes(), W: w, H: h}})
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func nearestQuad(c color.Color) int {
	quad := [4][3]int{{220, 30, 30}, {30, 200, 30}, {30, 60, 220}, {230, 200, 20}}
	r, g, b, _ := c.RGBA()
	rr, gg, bb := int(r>>8), int(g>>8), int(b>>8)
	best, bestD := -1, 1<<30
	for i, q := range quad {
		d := (rr-q[0])*(rr-q[0]) + (gg-q[1])*(gg-q[1]) + (bb-q[2])*(bb-q[2])
		if d < bestD {
			bestD, best = d, i
		}
	}
	return best
}

// TestSplitPageNoBleed renders a 2×2 resize split of a four-quadrant page and
// proves each tile shows ONLY its own quadrant's colour — i.e. the resize clip
// keeps the neighbouring quadrants (still present in the shared content stream)
// from bleeding in. This is the render-time guarantee a byte-level test can't
// reach; it needs a rasterizer, so it skips where poppler isn't installed.
func TestSplitPageNoBleed(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm (poppler) not installed; skipping render-based no-bleed check")
	}
	pdf := fourQuadrantPDF(t, 300, 400)
	out, err := SplitPage(pdf, 1, 2, 2, true) // 2×2, resize to full page
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "split.pdf")
	if err := os.WriteFile(src, out, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("pdftoppm", "-png", "-r", "36", src, filepath.Join(dir, "page")).Run(); err != nil {
		t.Fatalf("pdftoppm: %v", err)
	}

	// Tiles come out in reading order: page 1 = TL (red, quad 0), 2 = TR (green,
	// 1), 3 = BL (blue, 2), 4 = BR (yellow, 3).
	wantQuad := []int{0, 1, 2, 3}
	entries, _ := filepath.Glob(filepath.Join(dir, "page-*.png"))
	sort.Strings(entries) // pdftoppm names them page-1.png… in page order
	if len(entries) != 4 {
		t.Fatalf("rendered %d pages, want 4", len(entries))
	}
	for idx, want := range wantQuad {
		img := decodePNG(t, entries[idx])
		b := img.Bounds()
		// Sample the four corners and the centre; every sample must be this tile's
		// quadrant colour — any other colour means a neighbour bled through.
		pts := [][2]int{
			{b.Min.X + 2, b.Min.Y + 2}, {b.Max.X - 3, b.Min.Y + 2},
			{b.Min.X + 2, b.Max.Y - 3}, {b.Max.X - 3, b.Max.Y - 3},
			{(b.Min.X + b.Max.X) / 2, (b.Min.Y + b.Max.Y) / 2},
		}
		for _, p := range pts {
			if got := nearestQuad(img.At(p[0], p[1])); got != want {
				t.Errorf("tile page %d at (%d,%d): quadrant %d bled in, want only quadrant %d",
					idx+1, p[0], p[1], got, want)
			}
		}
	}
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}
