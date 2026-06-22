package pdfops

import "testing"

// multiQuadrantPDF builds an n-page PDF where every page is a four-quadrant
// w×h-point page (see fourQuadrantPDF). Used to exercise Crop's all-pages and
// page-selection paths and its page-count invariant.
func multiQuadrantPDF(t *testing.T, n int, w, h float64) []byte {
	t.Helper()
	doc := fourQuadrantPDF(t, w, h)
	for i := 1; i < n; i++ {
		more := fourQuadrantPDF(t, w, h)
		var err error
		if doc, err = Append(doc, more); err != nil {
			t.Fatal(err)
		}
	}
	return doc
}

// TestCropNoBleed crops a four-quadrant page to one quadrant and proves the
// output page shows ONLY that quadrant's colour (the clip keeps the rest of the
// page, still in the shared content stream, from bleeding in) and is sized exactly
// to the crop window — no standardization/padding, unlike SplitRegions.
func TestCropNoBleed(t *testing.T) {
	skipNoPoppler(t)
	pdf := fourQuadrantPDF(t, 300, 400)
	// Bottom-left-origin rect over the bottom-right quadrant (yellow, quad 3).
	out, err := Crop(pdf, [4]float64{150, 0, 300, 200}, nil)
	if err != nil {
		t.Fatal(err)
	}
	imgs := renderPDF(t, out, "crop")
	if len(imgs) != 1 {
		t.Fatalf("rendered %d pages, want 1", len(imgs))
	}
	img := imgs[0]
	for _, p := range cornersAndCentre(img.Bounds()) {
		if got := nearestQuad(img.At(p[0], p[1])); got != 3 {
			t.Errorf("crop at (%d,%d): quadrant %d bled in, want only 3 (yellow)", p[0], p[1], got)
		}
	}
	// 150×200pt at 36dpi → 75×100px; exact crop window, no padding.
	if dx, dy := img.Bounds().Dx(), img.Bounds().Dy(); dx > 78 || dx < 72 || dy > 103 || dy < 97 {
		t.Errorf("cropped page is %dx%d px, want ~75x100 (the crop window, not padded)", dx, dy)
	}
}

// TestCropAllPages crops every page of a 3-page document to the top-left quadrant
// and proves all pages come back cropped (red only) with the count unchanged.
func TestCropAllPages(t *testing.T) {
	skipNoPoppler(t)
	pdf := multiQuadrantPDF(t, 3, 300, 400)
	out, err := Crop(pdf, [4]float64{0, 200, 150, 400}, nil) // top-left quadrant (red, 0)
	if err != nil {
		t.Fatal(err)
	}
	imgs := renderPDF(t, out, "all")
	if len(imgs) != 3 {
		t.Fatalf("rendered %d pages, want 3", len(imgs))
	}
	for i, img := range imgs {
		for _, p := range cornersAndCentre(img.Bounds()) {
			if got := nearestQuad(img.At(p[0], p[1])); got != 0 {
				t.Errorf("page %d at (%d,%d): quadrant %d, want 0 (red)", i+1, p[0], p[1], got)
			}
		}
	}
}

// TestCropSelection crops only page 1 of a 2-page document and proves page 1 is
// cropped (red, narrower) while page 2 is left fully untouched (all four
// quadrants, full size).
func TestCropSelection(t *testing.T) {
	skipNoPoppler(t)
	pdf := multiQuadrantPDF(t, 2, 300, 400)
	out, err := Crop(pdf, [4]float64{0, 200, 150, 400}, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	imgs := renderPDF(t, out, "sel")
	if len(imgs) != 2 {
		t.Fatalf("rendered %d pages, want 2", len(imgs))
	}
	if imgs[0].Bounds().Dx() >= imgs[1].Bounds().Dx() {
		t.Errorf("cropped page 1 should be narrower than untouched page 2: %v vs %v",
			imgs[0].Bounds().Size(), imgs[1].Bounds().Size())
	}
	for _, p := range cornersAndCentre(imgs[0].Bounds()) {
		if got := nearestQuad(imgs[0].At(p[0], p[1])); got != 0 {
			t.Errorf("cropped page 1 at (%d,%d): quadrant %d, want 0 (red)", p[0], p[1], got)
		}
	}
	seen := map[int]bool{}
	b := imgs[1].Bounds()
	for _, p := range cornersAndCentre(b) {
		seen[nearestQuad(imgs[1].At(p[0], p[1]))] = true
	}
	if !seen[0] || !seen[1] || !seen[2] || !seen[3] {
		t.Errorf("page 2 should be untouched (all 4 quadrants present), saw %v", seen)
	}
}

// TestCropRotated is the rotation-correctness guard: the client measures the rect
// in pdf.js DISPLAY space, so the server must flatten /Rotate before cropping.
// Oracle: cropping a /Rotate-90 page to its FULL display rect must reproduce
// poppler's own (correct) rendering of that page.
func TestCropRotated(t *testing.T) {
	skipNoPoppler(t)
	base := fourQuadrantPDF(t, 300, 400)
	rotated, err := Rotate(base, nil, 90) // sets /Rotate 90 (metadata, not baked)
	if err != nil {
		t.Fatal(err)
	}
	full, err := Crop(rotated, [4]float64{0, 0, 400, 300}, nil) // 300×400 displays as 400×300
	if err != nil {
		t.Fatal(err)
	}
	truth := renderPDF(t, rotated, "ctruth") // poppler applies /Rotate correctly
	mine := renderPDF(t, full, "cmine")
	if len(truth) != 1 || len(mine) != 1 {
		t.Fatalf("expected truth=1, mine=1 pages, got truth=%d mine=%d", len(truth), len(mine))
	}
	tb, mb := truth[0].Bounds(), mine[0].Bounds()
	if tb.Dx() != mb.Dx() || tb.Dy() != mb.Dy() {
		t.Fatalf("size mismatch: truth %dx%d, mine %dx%d (rotation not flattened correctly)",
			tb.Dx(), tb.Dy(), mb.Dx(), mb.Dy())
	}
	for gy := 1; gy < 5; gy++ {
		for gx := 1; gx < 5; gx++ {
			x := tb.Min.X + tb.Dx()*gx/5
			y := tb.Min.Y + tb.Dy()*gy/5
			if nearestQuad(truth[0].At(x, y)) != nearestQuad(mine[0].At(x, y)) {
				t.Errorf("at (%d,%d): crop render disagrees with poppler's rotated render — rotation mishandled", x, y)
			}
		}
	}
}

// TestCropPageCountUnchanged proves Crop neither adds nor drops pages (no
// poppler needed) — the single-pass split→crop→merge must round-trip the count.
func TestCropPageCountUnchanged(t *testing.T) {
	pdf := multiQuadrantPDF(t, 4, 300, 400)
	out, err := Crop(pdf, [4]float64{50, 50, 250, 350}, nil)
	if err != nil {
		t.Fatal(err)
	}
	n, err := PageCount(out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("page count = %d, want 4 (crop must not add or drop pages)", n)
	}
}

// TestCropRejectsTinyRegion proves a degenerate (sub-point) crop window is
// rejected rather than producing an empty page.
func TestCropRejectsTinyRegion(t *testing.T) {
	pdf := fourQuadrantPDF(t, 300, 400)
	if _, err := Crop(pdf, [4]float64{10, 10, 10.5, 200}, nil); err == nil {
		t.Error("expected an error for a sub-point-wide crop region")
	}
}
