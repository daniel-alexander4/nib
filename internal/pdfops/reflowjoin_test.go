package pdfops

import (
	"math"
	"strings"
	"testing"
)

// The P08 phase-close review's typographic residue (/pending 798).

// glyphStarts is every glyph the runs draw, in content order, with the x it starts at — read from the runs
// themselves, never through `paragraphWords`, which is the reader under test.
func glyphStarts(runs []textRun) (text string, xs []float64) {
	var b strings.Builder
	for _, r := range runs {
		pos := r.x
		for _, g := range r.glyphs {
			pos += g.kern
			if len([]rune(g.text)) == 1 { // one glyph, one rune: the index into text is the index into xs
				b.WriteString(g.text)
				xs = append(xs, pos)
			}
			pos += g.advance
		}
	}
	return b.String(), xs
}

// TestAWordJoinedAcrossRunsKeepsTheAdjustmentBetweenThem — a `TJ` array that ENDS in a number moves the next
// run's first glyph, and that number is in no glyph's kern. A word drawn across the two runs was re-set
// without it: the letters after the seam moved by the adjustment, here 1.44 pt closer.
func TestAWordJoinedAcrossRunsKeepsTheAdjustmentBetweenThem(t *testing.T) {
	pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td [(The quick ja) -120] TJ (zzy fox jumps) Tj" +
		" T* (over the lazy dog and runs) Tj T* (away from here.) Tj ET")
	seam := func(pdf []byte) float64 {
		t.Helper()
		_, runs := layoutOf(t, pdf)
		text, xs := glyphStarts(runs)
		i := strings.Index(text, "jazzy")
		if i < 0 || len(text) != len(xs) {
			t.Fatalf("the page's glyphs read %q — no word drawn across the two runs", text)
		}
		return xs[i+2] - xs[i] // from the j to the first z, across the seam
	}
	before, beforeRuns := layoutOf(t, pdf)
	if len(beforeRuns) < 2 || math.Abs(beforeRuns[0].kernAfter-1.44) > 1e-6 {
		t.Fatalf("setup: the first run ends in an adjustment of %v pt, want 1.44 — the defect has no subject", beforeRuns[0].kernAfter)
	}
	if got := before.paragraphs[0].text(); !strings.HasPrefix(got, "The quick jazzy fox") {
		t.Fatalf("setup: the paragraph reads %q — the two runs are not one word", got)
	}
	want := seam(pdf)

	out, cause := reflowed(t, pdf, 0, "The quick jazzy fox jumps over the lazy dog and walks away from here.")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	if got := seam(out); math.Abs(got-want) > 1e-3 {
		t.Errorf("a kept word drawn across two runs was re-set with its second run %.3f pt from where it was "+
			"(j to z: %.3f pt before, %.3f pt after) — the adjustment that ends the first run was dropped",
			got-want, want, got)
	}
}
