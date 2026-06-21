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

// renderPDF rasterizes every page of pdf via poppler and returns the page images
// in page order.
func renderPDF(t *testing.T, pdf []byte, tag string) []image.Image {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, tag+".pdf")
	if err := os.WriteFile(src, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("pdftoppm", "-png", "-r", "36", src, filepath.Join(dir, tag)).Run(); err != nil {
		t.Fatalf("pdftoppm: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, tag+"-*.png"))
	sort.Strings(files)
	imgs := make([]image.Image, len(files))
	for i, f := range files {
		imgs[i] = decodePNG(t, f)
	}
	return imgs
}

func skipNoPoppler(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm (poppler) not installed; skipping render-based check")
	}
}

func cornersAndCentre(b image.Rectangle) [][2]int {
	return [][2]int{
		{b.Min.X + 2, b.Min.Y + 2}, {b.Max.X - 3, b.Min.Y + 2},
		{b.Min.X + 2, b.Max.Y - 3}, {b.Max.X - 3, b.Max.Y - 3},
		{(b.Min.X + b.Max.X) / 2, (b.Min.Y + b.Max.Y) / 2},
	}
}

// TestSplitRegionsNoBleed renders a region split of a four-quadrant page and
// proves each output page shows ONLY the colour of its selected quadrant — the
// per-region clip keeps the rest of the page (still in the shared content stream)
// from bleeding in.
func TestSplitRegionsNoBleed(t *testing.T) {
	skipNoPoppler(t)
	pdf := fourQuadrantPDF(t, 300, 400)
	// Bottom-left-origin point rects: top-left quadrant (red, quad 0) and
	// bottom-right quadrant (yellow, quad 3).
	out, err := SplitRegions(pdf, 1, [][4]float64{{0, 200, 150, 400}, {150, 0, 300, 200}})
	if err != nil {
		t.Fatal(err)
	}
	imgs := renderPDF(t, out, "page")
	// Non-destructive: page 1 is the original; the two regions are pages 2 and 3.
	if len(imgs) != 3 {
		t.Fatalf("rendered %d pages, want 3 (original + 2 regions)", len(imgs))
	}
	for i, want := range []int{0, 3} { // region pages 2,3 → red (0), yellow (3)
		img := imgs[i+1]
		for _, p := range cornersAndCentre(img.Bounds()) {
			if got := nearestQuad(img.At(p[0], p[1])); got != want {
				t.Errorf("region page %d at (%d,%d): quadrant %d bled in, want only %d",
					i+2, p[0], p[1], got, want)
			}
		}
	}
}

func whitish(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r>>8 > 200 && g>>8 > 200 && b>>8 > 200
}

// TestSplitRegionsStandardizePadding renders two differently-sized regions and
// proves the output is standardized: both pages come out the same size, the
// smaller region (red) is centred with WHITE padding at the corners, and the
// larger region (yellow) fills its page — with no neighbour-quadrant bleed.
func TestSplitRegionsStandardizePadding(t *testing.T) {
	skipNoPoppler(t)
	pdf := fourQuadrantPDF(t, 300, 400)
	// A: small region inside the top-left (red) quadrant; B: larger region inside
	// the bottom-right (yellow) quadrant. A=100×160, B=130×180 → standardized 130×180.
	out, err := SplitRegions(pdf, 1, [][4]float64{{20, 220, 120, 380}, {160, 10, 290, 190}})
	if err != nil {
		t.Fatal(err)
	}
	imgs := renderPDF(t, out, "pad") // [original, A, B]
	if len(imgs) != 3 {
		t.Fatalf("rendered %d pages, want 3", len(imgs))
	}
	a, b := imgs[1], imgs[2]
	if a.Bounds().Size() != b.Bounds().Size() {
		t.Fatalf("region pages not standardized: %v vs %v", a.Bounds().Size(), b.Bounds().Size())
	}
	ab, bb := a.Bounds(), b.Bounds()
	if got := nearestQuad(a.At((ab.Min.X+ab.Max.X)/2, (ab.Min.Y+ab.Max.Y)/2)); got != 0 {
		t.Errorf("small region centre quadrant %d, want 0 (red)", got)
	}
	for _, p := range [][2]int{{ab.Min.X + 2, ab.Min.Y + 2}, {ab.Max.X - 3, ab.Max.Y - 3}} {
		if !whitish(a.At(p[0], p[1])) {
			t.Errorf("small region corner (%d,%d) not white padding", p[0], p[1])
		}
	}
	if got := nearestQuad(b.At((bb.Min.X+bb.Max.X)/2, (bb.Min.Y+bb.Max.Y)/2)); got != 3 {
		t.Errorf("larger region centre quadrant %d, want 3 (yellow)", got)
	}
}

// TestSplitRegionsRotated is the rotation-correctness guard the rescrut required:
// the client measures rectangles in pdf.js DISPLAY space, but the server crops the
// content stream — which is wrong unless /Rotate is flattened first. Oracle: on a
// /Rotate-90 page, cropping to the FULL display rect must reproduce poppler's own
// (correct) rendering of that page. If normalization mishandled rotation, the two
// renderings would diverge.
func TestSplitRegionsRotated(t *testing.T) {
	skipNoPoppler(t)
	base := fourQuadrantPDF(t, 300, 400)
	rotated, err := Rotate(base, nil, 90) // sets /Rotate 90 (metadata, not baked)
	if err != nil {
		t.Fatal(err)
	}
	// A 300×400 page rotated 90° displays as 400×300.
	full, err := SplitRegions(rotated, 1, [][4]float64{{0, 0, 400, 300}})
	if err != nil {
		t.Fatal(err)
	}
	truth := renderPDF(t, rotated, "truth") // poppler applies /Rotate correctly
	mine := renderPDF(t, full, "mine")
	// Non-destructive: mine is [original-rotated, region]; the region is page 2.
	if len(truth) != 1 || len(mine) != 2 {
		t.Fatalf("expected truth=1, mine=2 pages, got truth=%d mine=%d", len(truth), len(mine))
	}
	region := mine[1]
	tb, mb := truth[0].Bounds(), region.Bounds()
	if tb.Dx() != mb.Dx() || tb.Dy() != mb.Dy() {
		t.Fatalf("size mismatch: truth %dx%d, mine %dx%d (rotation not flattened correctly)",
			tb.Dx(), tb.Dy(), mb.Dx(), mb.Dy())
	}
	// Sample an interior grid; every point's quadrant colour must match the oracle.
	for gy := 1; gy < 5; gy++ {
		for gx := 1; gx < 5; gx++ {
			x := tb.Min.X + tb.Dx()*gx/5
			y := tb.Min.Y + tb.Dy()*gy/5
			if nearestQuad(truth[0].At(x, y)) != nearestQuad(region.At(x, y)) {
				t.Errorf("at (%d,%d): split render disagrees with poppler's rotated render — rotation mishandled", x, y)
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
