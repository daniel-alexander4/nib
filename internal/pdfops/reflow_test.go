package pdfops

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// rebreak breaks words at the paragraph's line measures with space between them, through the one line breaker — the
// breaker as the rewrite calls it, over the words the paragraph already has. A test instrument.
func rebreak(words []reflowWord, measures []float64, space float64) [][]reflowWord {
	return breakAt(words, measures, func(w reflowWord) float64 { return w.width }, space)
}

// lineTexts renders broken lines as their words joined by a space — what a reader of the broken paragraph sees.
func lineTexts(lines [][]reflowWord) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		for j, w := range l {
			if j > 0 {
				out[i] += " "
			}
			out[i] += w.text()
		}
	}
	return out
}

// TestAnUneditedParagraphRebreaksWhereItWasBroken — `PLAN-text-reflow.md` P06.S03: a paragraph re-broken by the one line
// breaker, at its own measure, with its own glyph advances and space, reproduces the line breaks it was set with — on
// the hand-checked corpus, whose converters set ragged-right greedily. Anything else would mean a reflow that changes
// nothing moves words between lines.
//
// **The stimulus is asserted**: the corpus must hold multi-line paragraphs, or a paragraph of one line reproduces
// trivially and the breaker was never asked anything.
func TestAnUneditedParagraphRebreaksWhereItWasBroken(t *testing.T) {
	multi := 0
	var md strings.Builder
	for i, n := range []int{9, 23, 41, 6, 64, 17, 30} {
		md.WriteString(strings.Repeat("Words of varied length make a paragraph wrap unevenly, ", n/5+1))
		if i%2 == 0 {
			md.WriteString("with **bold** and *italic* spans inside it")
		}
		md.WriteString(".\n\n")
	}
	wrapped, err := ConvertDocToPDF([]byte(md.String()), ".md")
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range append(runCorpus(t), runCorpusDoc{"seven wrapped Markdown paragraphs", wrapped}) {
		ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		for p := 1; p <= ctx.PageCount; p++ {
			l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
			if err != nil {
				t.Fatal(err)
			}
			for _, para := range l.paragraphs {
				lines, space, cause := paragraphWords(para)
				if cause != "" {
					t.Errorf("%s p%d: %q cannot be reflowed: %s", doc.name, p, para.text(), cause)
					continue
				}
				var words []reflowWord
				for _, ln := range lines {
					words = append(words, ln...)
				}
				got := lineTexts(rebreak(words, lineMeasures(lines, space), space))
				want := lineTexts(lines)
				if len(want) > 1 {
					multi++
				}
				if strings.Join(got, "\n") != strings.Join(want, "\n") {
					t.Errorf("%s p%d: re-broken at its own measure\n  got  %q\n  want %q", doc.name, p, got, want)
				}
			}
		}
	}
	if multi < 7 {
		t.Errorf("%d multi-line paragraphs — the seven wrapped ones alone should give seven, so the breaker was barely asked", multi)
	}
	t.Logf("%d multi-line paragraphs reproduced", multi)
}

// TestHowOftenARealProducersParagraphRebreaksInPlace is a MEASUREMENT, not an assertion: over the real-producer corpus, how
// many multi-line paragraphs the greedy breaker reproduces at their own measure. A producer that justifies, hyphenates
// or breaks by another rule will not reproduce, and that is P08's to handle — the number says how much of the real
// world S03's greedy re-break already matches. Skips without the corpus.
func TestHowOftenARealProducersParagraphRebreaksInPlace(t *testing.T) {
	corp := externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers"))
	if corp.absent != "" {
		t.Skip("SKIP (not a measurement): " + corp.absent)
	}
	causes := map[string]int{}
	same, differ := 0, 0
	for _, doc := range corp.docs {
		ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
		if err != nil {
			continue
		}
		for p := 1; p <= ctx.PageCount; p++ {
			l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
			if err != nil {
				causes["unreadable page"]++
				continue
			}
			for _, para := range l.paragraphs {
				if len(para.lines) < 2 {
					continue
				}
				lines, space, cause := paragraphWords(para)
				if cause != "" {
					causes[cause]++
					continue
				}
				var words []reflowWord
				for _, ln := range lines {
					words = append(words, ln...)
				}
				if strings.Join(lineTexts(rebreak(words, lineMeasures(lines, space), space)), "\n") == strings.Join(lineTexts(lines), "\n") {
					same++
				} else {
					differ++
				}
			}
		}
	}
	t.Logf("real producers, multi-line paragraphs: %d re-break in place, %d do not, refused by cause %v", same, differ, causes)
}

// glyphsOf builds glyphs for s at advance per character, with kern before each glyph after the first taken from kerns
// (by index, 0 when absent).
func glyphsOf(s string, advance float64, kerns map[int]float64) []runGlyph {
	var out []runGlyph
	for i, r := range s {
		out = append(out, runGlyph{code: []byte{byte(r)}, text: string(r), decoded: true, widthSrc: widthFromWidths,
			fontWidth: advance * 100, advance: advance, kern: kerns[i]})
	}
	return out
}

func glyphRun(x float64, g []runGlyph) textRun {
	var w float64
	for _, gl := range g {
		w += gl.kern + gl.advance
	}
	return textRun{x: x, width: w, size: 10, decoded: true, widthSrc: widthFromWidths, codes: len(g), glyphs: g}
}

// TestAParagraphIsCutIntoWordsByItsOwnRules — the three word rules, and the space, on a hand-built line whose answer is
// derived by hand; the generated corpus draws uniform spaces with no in-word kerns and no word gaps between runs, so none
// of these was reached there (each was a surviving mutation).
func TestAParagraphIsCutIntoWordsByItsOwnRules(t *testing.T) {
	// "ab cd" in one run with spaces of 2 then 3; a 0.5 kern inside "cd"; then a SECOND run 10 to the right with no space
	// glyph ("ef"), and a THIRD run abutting it ("g"): a gap of 10 separates words, a gap of 0 does not.
	first := glyphsOf("ab cd", 1, map[int]float64{4: 0.5})
	first[2].advance = 2 // the space glyph
	second := append(glyphsOf(" ", 3, nil), glyphsOf("x", 1, nil)...)
	second[1].kern = 0.7 // before "x": it widens the gap in front of the word, not the word
	r1 := glyphRun(0, append(first, second...))
	r2 := glyphRun(r1.width+10, glyphsOf("ef", 1, nil))
	r3 := glyphRun(r2.x+r2.width, glyphsOf("g", 1, nil))
	para := textParagraph{lines: []textLine{{runs: []textRun{r1, r2, r3}}}}
	lines, space, cause := paragraphWords(para)
	if cause != "" {
		t.Fatalf("refused: %s", cause)
	}
	got := lineTexts(lines)
	if len(got) != 1 || got[0] != "ab cd x efg" {
		t.Errorf("words %q, want one line \"ab cd x efg\" — a gap between runs separates words, an abutting run does not", got)
	}
	if len(lines[0]) != 4 || lines[0][1].width != 2.5 {
		t.Errorf("\"cd\" is 1 + (0.5 + 1) = 2.5 wide with its inner kern; got %v", lines)
	}
	if lines[0][2].width != 1 {
		t.Errorf("\"x\" is 1 wide; the 0.7 kern before it belongs to the gap, got %v", lines[0][2].width)
	}
	if space != 3 {
		t.Errorf("the gaps are 2, 3 and 10, and the paragraph's space is their median, 3; got %v", space)
	}
}

// TestAParagraphItCannotReadFaithfullyNamesWhy — paragraphWords' own refusals, each on a run built to reach it: text drawn
// inside a form XObject (rewriting the page stream would describe the `Do`), a turned baseline, a width measured from
// nothing (law 2), and a code that decoded to nothing.
func TestAParagraphItCannotReadFaithfullyNamesWhy(t *testing.T) {
	base := func() textRun { return glyphRun(0, glyphsOf("ab", 1, nil)) }
	for _, c := range []struct {
		want string
		edit func(*textRun)
	}{
		{"text-in-form", func(r *textRun) { r.inForm = true }},
		{"rotated", func(r *textRun) { r.rotated = true }},
		{"no-widths", func(r *textRun) { r.widthSrc = widthNone }},
		{"undecoded", func(r *textRun) { r.decoded = false }},
		{"glyphs-not-kept", func(r *textRun) { r.glyphs = nil }},
	} {
		r := base()
		c.edit(&r)
		if _, _, cause := paragraphWords(textParagraph{lines: []textLine{{runs: []textRun{r}}}}); cause != c.want {
			t.Errorf("want %q, got %q", c.want, cause)
		}
	}
	// And the control: the same run unedited is read, and a paragraph with no gap in it cannot say how wide its space is.
	if _, _, cause := paragraphWords(textParagraph{lines: []textLine{{runs: []textRun{base()}}}}); cause != "no-space-width" {
		t.Errorf("a one-word paragraph has no space to measure; got %q", cause)
	}
}
