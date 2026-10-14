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

// TestARaisedFigureStandsAtItsWord — /pending 871, with the numbers of the pages that showed it: a smaller run above a
// line's baseline is a raised figure only where the line ends. A 12pt table cell inside a 63pt stamp drawn across the
// table (`acrobat/fda-156618.pdf` p2), a form's 8pt label 7pt past a 10pt one (`designer/irs-f1040.pdf` p1) and a
// contents entry 8pt past a 31pt title (`antennahouse/irs-p15.pdf` p1) each read as raised on the larger text.
func TestARaisedFigureStandsAtItsWord(t *testing.T) {
	for _, c := range []struct {
		name string
		runs []textRun
		want []string
	}{
		{"a cell inside a stamp drawn across it", []textRun{run("REVOCADO", 212.31, 249.26, 343.49, 63.41), run("1 vial ", 286.85, 272.57, 31.16, 12)},
			[]string{"1 vial", "REVOCADO"}},
		{"a label past a larger label", []textRun{run("Digital Assets", 36, 501.9, 62, 10), run("At ", 105, 505.5, 9.8, 8)},
			[]string{"At", "Digital Assets"}},
		{"a contents entry past a title", []textRun{run("Employer’s Tax ", 42, 635.9, 211, 31), run("1. Employer Identification Number (EIN)", 261.06, 650.6, 170, 10)},
			[]string{"1. Employer Identification Number (EIN)", "Employer’s Tax"}},
		// Still raised: a mark a fifth of an em past its word (`acrobat/fda-71021.pdf` p11), and one set back over the
		// space its word's run ends with (`acrobat/fda-156618.pdf` p2).
		{"a mark a fifth of an em past its word", []textRun{run("characteristics", 72, 198.8, 70, 12), run("13", 144.4, 204.3, 8, 8)},
			[]string{"characteristics 13"}},
		{"a mark set back over a space", []textRun{run("3mL ", 426.8, 544.15, 36.14, 12), run("* ", 456.55, 548.11, 5.36, 8.04)},
			[]string{"3mL *"}},
	} {
		var got []string
		for _, l := range lineSegments(c.runs) {
			got = append(got, l.text)
		}
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s: the lines read %q, want %q", c.name, got, c.want)
		}
	}
}

// TestAMarkThatOpensItsLineIsOnIt — /pending 871: a footnote's own number, set smaller and above the baseline AHEAD
// of its text (`acrobat/fda-170357.pdf` p3: a 6.5pt "2 " at y 147.0 ending at x 76.9, then 10pt text at y 143.5 from
// x 77.8), is the start of that line, and the footnote is one paragraph. It was a line, and a paragraph, of its own.
func TestAMarkThatOpensItsLineIsOnIt(t *testing.T) {
	note := func(mark textRun) []textRun {
		return []textRun{
			run("The body of the page, set at twelve points, above the notes.", 72, 300, 320, 12),
			mark, run("Google. 2019 Map of 600 Perdue Ave., Richmond, VA 23234.  Retrieved", 77.8, 143.5, 330, 10),
			run("from Google Maps.", 72, 132, 80, 10),
		}
	}
	lines := lineSegments(note(run("2 ", 72, 147.0, 4.9, 6.5)))
	if len(lines) != 3 || lines[1].text != "2 Google. 2019 Map of 600 Perdue Ave., Richmond, VA 23234.  Retrieved" || lines[1].y != 143.5 || lines[1].x0 != 72 || lines[1].size != 10 {
		t.Fatalf("the lines read %+v", lines)
	}
	if l := groupRuns(note(run("2 ", 72, 147.0, 4.9, 6.5))); len(l.paragraphs) != 2 || len(l.paragraphs[1].lines) != 2 {
		t.Errorf("the footnote reads as %q, want one paragraph of two lines after the body", paragraphTexts(l))
	}
	for name, mark := range map[string]textRun{
		"text of the line's own size":        run("2 ", 72, 147.0, 4.9, 10),
		"small text more than half an em up": run("2 ", 72, 149.0, 4.9, 6.5),
		"small text below the line":          run("2 ", 72, 139.5, 4.9, 6.5),
		"small text ending well short of it": run("2 ", 70, 147.0, 4.9, 6.5),
		"more than a mark":                   run("Note ", 72, 147.0, 4.9, 6.5),
	} {
		alone := false
		for _, l := range lineSegments(note(mark)) {
			alone = alone || l.text == strings.TrimSpace(mark.text)
		}
		if !alone {
			t.Errorf("%s opened the line: %+v", name, lineSegments(note(mark)))
		}
	}
}

// TestARuleDrawnAsAGlyphJoinsNoText — /pending 871 (`acrobat/fda-170357.pdf` p42): a signature rule drawn as one 72pt
// hyphen, 85pt right of a 9.8pt line and 14pt above its baseline, is within 0.3 em and 1.5 em of it by its OWN em. It
// joined the line and gave it its size. An ordinary dash, and large text that is not a rule, still join.
func TestARuleDrawnAsAGlyphJoinsNoText(t *testing.T) {
	title := run("Vice President Operations ", 71.99, 522.51, 119.07, 9.8)
	for _, c := range []struct {
		name string
		runs []textRun
		want []string
	}{
		{"a 72pt hyphen right of a line", []textRun{title, run("-", 275.82, 537, 23.98, 72)}, []string{"-", "Vice President Operations"}},
		{"a 72pt hyphen left of a line", []textRun{run("-", 20, 537, 23.98, 72), title}, []string{"-", "Vice President Operations"}},
		{"a 30pt em dash", []textRun{title, run("—", 195, 522.51, 30, 30)}, []string{"Vice President Operations", "—"}},
		{"a dash of the line's size", []textRun{title, run("- 2024", 195, 522.51, 30, 9.8)}, []string{"Vice President Operations - 2024"}},
		{"a large letter", []textRun{title, run("A", 195, 522.51, 20, 30)}, []string{"Vice President Operations A"}},
		{"a hyphen at twice the size", []textRun{title, run("-", 195, 522.51, 6, 19.6)}, []string{"Vice President Operations -"}},
	} {
		var got []string
		for _, l := range lineSegments(c.runs) {
			got = append(got, l.text)
		}
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s: the lines read %q, want %q", c.name, got, c.want)
		}
	}
}

// TestARunDoesNotContinueALineAcrossAGutter — /pending 871, with the numbers of the page that showed it
// (`designer/irs-fw9.pdf` p4): a two-column table whose left cell's text ran to within 1.4 em of the right cell's, so
// "be reported and direct sales over" and "1 through 5." read as one line. Three rows of the table are already parted
// (their left text is short), and that is the evidence the fourth is parted on. Each thing the rule asks for is taken
// away in turn, and without it the line is joined as it was.
func TestARunDoesNotContinueALineAcrossAGutter(t *testing.T) {
	parted := []textRun{
		run("•  Broker transactions", 38, 400.26, 80, 8), run("are exempt only for sales of", 169.60, 400.26, 101.19, 8),
		run("•  Interest payments", 38, 385.26, 75, 8), run("All exempt payees except", 169.60, 385.26, 90, 8),
		run("•  Barter exchange transactions", 38, 370.26, 115.71, 8), run("Exempt payees 1 through 4.", 169.60, 370.26, 100.78, 8),
	}
	left, right := run("be reported and direct sales over ", 38, 340.26, 120.61, 8), run("1 through 5.", 169.60, 338.40, 43.28, 8)
	const joined, apart = "be reported and direct sales over 1 through 5.", "be reported and direct sales over|1 through 5."
	with := func(rs []textRun, more ...textRun) []textRun { return append(append([]textRun(nil), rs...), more...) }
	for _, c := range []struct {
		name string
		runs []textRun
		want string
	}{
		{"three rows parted at the edge", with(parted, left, right), apart},
		{"only two rows parted there", with(parted[:4], left, right), joined},
		{"only two rows parted there, and two at another edge", with(parted[:4], left, right,
			run("Code", 300, 400.26, 20, 8), run("Code", 300, 385.26, 20, 8)), joined},
		// Under a 20pt title half an em is 10pt for the page's largest pair of lines; for these 8pt lines it is 4pt, and
		// the text to their left stands 6pt higher: a label above a hanging line, not a cell beside it.
		{"lines at the edge whose text to the left is on the line above", with([]textRun{parted[1], parted[3], parted[5]}, left, right,
			run("Title", 38, 500, 60, 20), run("Label", 38, 406.26, 30, 8), run("Label", 38, 391.26, 30, 8), run("Label", 38, 376.26, 30, 8)), joined},
		{"lines that begin at the edge with nothing to their left", with([]textRun{parted[1], parted[3], parted[5]}, left, right), joined},
		{"a line drawn across the edge just below", with(parted, left, right,
			run("Payments made in settlement of payment card or third-party network transactions", 38, 329.26, 250, 8)), joined},
		{"a line drawn across the edge well below", with(parted, left, right,
			run("Payments made in settlement of payment card or third-party network transactions", 38, 300.26, 250, 8)), apart},
		{"one word ahead of the gap", with(parted, run("Note: ", 38, 340.26, 120.61, 8), right), "Note: 1 through 5."},
		{"a gap no wider than a space after a sentence", with(parted, run("be reported and direct sales over ", 38, 340.26, 127, 8), right), joined},
	} {
		var got []string
		for _, l := range lineSegments(c.runs) {
			if l.y < 345 && l.y > 335 {
				got = append(got, l.text)
			}
		}
		if strings.Join(got, "|") != c.want {
			t.Errorf("%s: the row reads %q, want %q", c.name, strings.Join(got, "|"), c.want)
		}
	}
}

// TestALeaderJoinsItsPageNumberAcrossAGutter — /pending 871, with the numbers of the page that showed it
// (`antennahouse/irs-p15.pdf` p8): a contents list sets most of its page numbers past joinGapEm of their titles' dots, so
// the numbers' left edge is a gutter, and the gutter parted the one number set within joinGapEm from its title. A line
// that ends in a leader is not parted from what the leader leads to; the same line without one still is.
func TestALeaderJoinsItsPageNumberAcrossAGutter(t *testing.T) {
	list := []textRun{
		run("Verify work eligibility of new employees .......", 64.3, 671.1, 200, 9), run("5", 300, 671.1, 5, 9),
		run("social security cards ....................", 64.3, 646.1, 180, 9), run("16", 300, 646.1, 10, 9),
		run("Ask employees for Form W-4 ..............", 64.3, 633.6, 190, 9), run("24", 300, 633.6, 10, 9),
	}
	number := run("25", 300, 597.6, 10, 9)
	for _, c := range []struct {
		name, title, want string
	}{
		{"a leader of dots", "withholding ..............................", "withholding .............................. 25"},
		{"a leader of dots set apart by spaces", "withholding . . . . . . . . . .", "withholding . . . . . . . . . . 25"},
		{"a leader of dots, then a space", "withholding .............................. ", "withholding .............................. 25"},
		{"no leader", "withholding and the taxes withheld with it", "withholding and the taxes withheld with it|25"},
		{"an ellipsis, which is not a leader", "withholding and the taxes withheld . . .", "withholding and the taxes withheld . . .|25"},
		{"dots inside the line and none at its end", "withholding .......... and the taxes", "withholding .......... and the taxes|25"},
	} {
		var got []string
		for _, l := range lineSegments(append(append([]textRun(nil), list...), run(c.title, 64.3, 597.6, 223.5, 9), number)) {
			if l.y == 597.6 {
				got = append(got, l.text)
			}
		}
		if strings.Join(got, "|") != c.want {
			t.Errorf("%s: the row reads %q, want %q", c.name, strings.Join(got, "|"), c.want)
		}
	}
}

// TestAHeadingInTheNextColumnDoesNotJoinTheLineBesideIt — /pending 871 (`antennahouse/irs-p15.pdf` p11): two 10pt
// columns 18pt apart are parted by joinGapEm, but the gap is measured in ems of the LARGER run, so a 14pt heading
// opening the right-hand column joined the body line beside it — and one line across both made the page one column,
// its 28 paragraphs read as 88.
func TestAHeadingInTheNextColumnDoesNotJoinTheLineBesideIt(t *testing.T) {
	var runs []textRun
	for _, y := range []float64{300, 288.5, 272.1, 260.6, 249.1, 237.6} {
		runs = append(runs, run("ited income, social security, and Medicare taxes. You ", 42, y, 255, 10))
	}
	for _, y := range []float64{300, 288.5, 253, 241.5} {
		runs = append(runs, run("Forms W-4 claiming exemption from withholding expire.", 315, y, 255, 10))
	}
	runs = append(runs, run("On February 16", 315, 272.1, 100, 14))
	l := groupRuns(runs)
	if l.columns != 2 || l.unsupported != "" {
		t.Errorf("the page reads as %d column(s), unsupported %q; want two columns: %q", l.columns, l.unsupported, paragraphTexts(l))
	}
	found := false
	for _, s := range lineSegments(runs) {
		found = found || (s.text == "On February 16" && s.x0 == 315)
	}
	if !found {
		t.Errorf("the heading is not a line of its own")
	}
}

// TestAPageWithNoTextStillHasItsBoxAndAnnotations —/pending 794 (pdfops I2): reflow's reader gives a page that draws
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
