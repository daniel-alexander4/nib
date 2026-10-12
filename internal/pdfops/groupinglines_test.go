package pdfops

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// TestALineIsTheSameInWhateverOrderThePageDrawsIt — /pending 790, with the numbers of the page that showed it
// (`antennahouse/irs-p15.pdf` p16): a 12pt bullet at y 464.355 beside its 10pt text at 464.838, and the other column's
// line at 460.838. The bullet is within 0.3 em of both and they are not within it of each other, so the old sort by
// "shares a baseline" answered by draw order: with the left column's line drawn LAST — where a re-set paragraph's
// lines are — it was joined to the bullet's line, 279pt to its right.
func TestALineIsTheSameInWhateverOrderThePageDrawsIt(t *testing.T) {
	bullet := run("•", 321.005, 464.355, 4.692, 12)
	right := run("A partnership, even if the individual’s spouse is a part-", 331.997, 464.838, 234.970, 10)
	left := run("ship. Generally, filing as a qualified joint venture won’t in-", 42.002, 460.838, 254.974, 10)
	above := run("employees using the EIN of that spouse’s sole proprietor-", 41.999, 472.338, 254.976, 10)
	for name, runs := range map[string][]textRun{
		"as the producer drew it":       {above, left, bullet, right},
		"the left line drawn last":      {above, bullet, right, left},
		"the left line between the two": {bullet, left, right, above},
		"bottom to top":                 {left, right, bullet, above},
	} {
		var got []string
		for _, l := range lineSegments(runs) {
			got = append(got, fmt.Sprintf("%.0f %s", l.x0, l.text))
		}
		want := []string{
			"42 employees using the EIN of that spouse’s sole proprietor-",
			"321 • A partnership, even if the individual’s spouse is a part-",
			"42 ship. Generally, filing as a qualified joint venture won’t in-",
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s: the lines read\n  %q\nwant\n  %q", name, got, want)
		}
	}
	// And the page is two columns of whole paragraphs either way.
	l := groupRuns([]textRun{above, bullet, right, left})
	if len(l.paragraphs) != 2 || l.columns != 2 || len(l.paragraphs[0].lines) != 2 {
		t.Errorf("the page reads as %d paragraph(s) in %d column(s): %q", len(l.paragraphs), l.columns, paragraphTexts(l))
	}
}

// TestRunsAtOneXKeepTheOrderTheyAreDrawnIn — Acrobat draws a run-in heading and the sentence after it from the same x
// (`acrobat/fda-176439.pdf` p9, "Note:" then "This means that…"); only the order they are drawn in says which is read
// first, and a baseline differing in its last bit must not reverse them.
func TestRunsAtOneXKeepTheOrderTheyAreDrawnIn(t *testing.T) {
	segs := lineSegments([]textRun{
		run("Note:", 108, 363.72, 26.57, 12),
		run("This means that some businesses may qualify as a ", 108, 363.7200001, 274.56, 12),
		run("small business", 382.56, 363.72, 73.2, 12),
	})
	if len(segs) != 1 || segs[0].text != "Note:This means that some businesses may qualify as a small business" {
		t.Errorf("the line read as %+v", segs)
	}
}

// TestAParagraphIsNotSplitAtAWordDrawnInTwoRuns — /pending 794 (the P08 phase-close review's pdfops W1): a line stops
// 30pt short and the next begins with a 40pt word that would not have fit, so the two are one paragraph — however many
// runs the word is drawn in. Measured from its first run alone the word was 8pt, "would have fit", and the paragraph
// split in two.
func TestAParagraphIsNotSplitAtAWordDrawnInTwoRuns(t *testing.T) {
	first := run("A first line of a ragged paragraph that reaches the edge", 72, 700, 228, 10)
	second := run("and a second line that stops short of it mmmm", 72, 688, 198, 10)
	whole := []textRun{first, second, run("Whatever ends the paragraph.", 72, 676, 130, 10)}
	split := []textRun{first, second, run("W", 72, 676, 8, 10), run("hatever", 80, 676, 32, 10), run(" ends the paragraph.", 112, 676, 90, 10)}
	glyphs := []textRun{first, second}
	for i, c := range "Whatever" {
		glyphs = append(glyphs, run(string(c), 72+5*float64(i), 676, 5, 10))
	}
	glyphs = append(glyphs, run("ends the paragraph.", 117, 676, 85, 10))
	for name, runs := range map[string][]textRun{"one run a line": whole, "the first word in two styles": split, "a run a glyph": glyphs} {
		if l := groupRuns(runs); len(l.paragraphs) != 1 {
			t.Errorf("%s: %d paragraphs, want 1: %q", name, len(l.paragraphs), paragraphTexts(l))
		}
	}
	// The rule still fires: a next line whose first WORD is short would have fit, and begins a paragraph.
	short := []textRun{first, second, run("I", 72, 676, 3, 10), run("t", 75, 676, 3, 10), run(" ends here.", 78, 676, 50, 10)}
	if l := groupRuns(short); len(l.paragraphs) != 2 {
		t.Errorf("a short first word drawn in two runs: %d paragraphs, want 2: %q", len(l.paragraphs), paragraphTexts(l))
	}
}

// TestARaisedFigureStaysOnItsLine — /pending 794 (pdfops W2): a 6.5pt footnote mark raised a third of an em on 10pt
// text — a word processor's default — is on its line, and the sentence it marks is one paragraph. It was a paragraph of
// its own, read first, with the sentence cut in two around it. What is not a raised figure still is not on the line.
func TestARaisedFigureStaysOnItsLine(t *testing.T) {
	page := func(mark textRun) []textRun {
		return []textRun{
			run("A claim that was made", 72, 700, 100, 10), mark, run(" and then discussed at length", 176, 700, 124, 10),
			run("in a second line of the same paragraph.", 72, 688, 170, 10),
		}
	}
	for _, c := range []struct {
		name string
		mark textRun
		want []string
	}{
		{"within 0.3 em, as before", run("1", 172, 702.9, 3.6, 6.5),
			[]string{"A claim that was made1 and then discussed at length in a second line of the same paragraph."}},
		{"raised a third of an em", run("1", 172, 703.3, 3.6, 6.5),
			[]string{"A claim that was made1 and then discussed at length in a second line of the same paragraph."}},
		{"raised as TeX raises an exponent", run("1", 172, 703.63, 3.6, 7),
			[]string{"A claim that was made1 and then discussed at length in a second line of the same paragraph."}},
	} {
		if got := paragraphTexts(groupRuns(page(c.mark))); strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s: the page reads %q, want %q", c.name, got, c.want)
		}
	}
	for _, c := range []struct {
		name string
		mark textRun
	}{
		{"text of the line's own size", run("1", 172, 703.3, 3.6, 10)},
		{"small text more than half an em up", run("1", 172, 705.2, 3.6, 6.5)},
		{"small text lowered a third of an em", run("1", 172, 696.7, 3.6, 6.5)},
	} {
		// A 20pt heading on the page, so that how far up a line is looked for is not what keeps the mark off this one.
		lines := lineSegments(append(page(c.mark), run("A heading", 72, 760, 90, 20)))
		alone := false
		for _, l := range lines {
			alone = alone || l.text == "1"
		}
		if !alone {
			t.Errorf("%s joined the line: %+v", c.name, lines)
		}
	}
}

// TestAPageWithNoTextStillHasItsBoxAndAnnotations — /pending 794 (pdfops I2): reflow's reader gives a page that draws
// no glyph the page box and annotation boxes it gives every other page, so a destination read from it is not measured
// against zeroes.
func TestAPageWithNoTextStillHasItsBoxAndAnnotations(t *testing.T) {
	content := "0 0 1 rg 100 100 50 50 re f"
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R /Annots [6 0 R] >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		6: "<< /Type /Annot /Subtype /Link /Rect [72 680 200 700] /Border [0 0 0] " +
			"/A << /Type /Action /S /URI /URI (https://example.invalid/) >> >>",
	})
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !l.noText || len(l.paragraphs) != 0 {
		t.Fatalf("the fixture reads as text: %+v", l)
	}
	if l.box != [4]float64{0, 0, 612, 792} {
		t.Errorf("the page box reads %v, want the media box", l.box)
	}
	if len(l.annots) != 1 || l.annots[0] != [4]float64{72, 680, 200, 700} {
		t.Errorf("the annotation boxes read %v, want the link's", l.annots)
	}
	if len(l.marks) == 0 {
		t.Errorf("the painted square is not among the page's marks")
	}
}
