package pdfops

import (
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// rebreak breaks words at the paragraph's line measures with space between them, through the one line breaker — the
// breaker as the rewrite calls it, over the words the paragraph already has. A test instrument.
func rebreak(words []reflowWord, measures []float64, space wordSpacer) [][]reflowWord {
	return breakAt(words, measures, func(w reflowWord) float64 { return w.width },
		func(w reflowWord) float64 { return space(w.face, w.tfSize, lastSpacing(w.spacing)) }, nil, nil)
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
				got := lineTexts(rebreak(words, lineMeasures(lines, spacerOf(lines, space)), spacerOf(lines, space)))
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

// rebreakAsProduction breaks paragraph pi's own words as `reflowParagraphIn` breaks an edit's — the alignment read, a
// justified paragraph's words given one word spacing (`withWordSpacing`, P08.S03's C3), the spacer and the measures
// taken from THOSE lines, a centred paragraph its width about the axis, and a justified paragraph a line of which
// cannot be set within the flush edge broken again AT the edge — and says how the lines then read, how they read on
// the page, and whether the paragraph is justified. A test instrument: it follows `reflowParagraphIn` step for step
// and must be kept beside it. It rejoins no hyphenated break and keeps none (P08.S07): the words are the paragraph's
// own, as read.
//
// edgeFirst is /pending 797's proposed remedy, measured and not adopted: a justified paragraph broken at its flush
// edge from the start, at its natural spaces, never at its own measure.
func rebreakAsProduction(l pageLayout, pi int, edgeFirst bool) (got, want []string, justified bool, cause string) {
	lines, gap, cause := paragraphWords(l.paragraphs[pi])
	if cause != "" {
		return nil, nil, false, cause
	}
	want = lineTexts(lines)
	align := paragraphAlignment(l, pi, lines)
	if align.justified {
		tw := lastSpacing(lines[len(lines)-1][len(lines[len(lines)-1])-1].spacing).tw
		if align.continues {
			for _, ln := range lines {
				tw = math.Min(tw, lastSpacing(ln[len(ln)-1].spacing).tw)
			}
		}
		lines = withWordSpacing(lines, tw)
		align = paragraphAlignment(l, pi, lines)
	}
	space := spacerOf(lines, gap)
	var words []emitWord
	var texts []string
	for _, ln := range lines {
		for _, w := range ln {
			words = append(words, emitWord{face: w.face, tfSize: w.tfSize, spacing: w.spacing, width: w.width, seq: len(words) + 1})
			texts = append(texts, w.text())
		}
	}
	width := func(w emitWord) float64 { return w.width }
	atEdge := func() [][]emitWord {
		edge := make([]float64, len(lines))
		for i, ln := range lines {
			edge[i] = align.edge - ln[0].startX
		}
		return breakAt(words, edge, width,
			func(w emitWord) float64 { return space(w.face, w.tfSize, lastSpacing(w.spacing)) + align.lastDelta }, nil, nil)
	}
	var broken [][]emitWord
	if edgeFirst && align.justified {
		broken = atEdge()
		if _, fits := justifiedLines(align, broken, lines, space); !fits {
			return nil, want, true, causeWordTooWide
		}
	} else {
		measures := paragraphMeasures(l, pi, lines, space)
		if align.centred {
			for i := range measures {
				measures[i] = align.width
			}
		}
		broken = breakAt(words, measures, width, func(w emitWord) float64 { return space(w.face, w.tfSize, lastSpacing(w.spacing)) }, nil, nil)
		if _, fits := justifiedLines(align, broken, lines, space); !fits {
			broken = atEdge()
			if _, fits = justifiedLines(align, broken, lines, space); !fits {
				return nil, want, align.justified, causeWordTooWide
			}
		}
	}
	got = make([]string, len(broken))
	for i, ln := range broken {
		for j, w := range ln {
			if j > 0 {
				got[i] += " "
			}
			got[i] += texts[w.seq-1]
		}
	}
	return got, want, align.justified, ""
}

// TestHowOftenARealProducersParagraphRebreaksInPlace is a MEASUREMENT, not an assertion: over the real-producer corpus, how
// many multi-line paragraphs re-break where they were broken when nothing in them changes — so an edit confined to one
// line moves no word on another. Skips without the corpus.
//
// **Through PRODUCTION's path** (`rebreakAsProduction`, /pending 797). It broke with `spacerOf` over the lines as read,
// which is the path S03 shipped and not the one production has taken since S03's own C3 gave a justified paragraph one
// word spacing before its spacer is read: the census reported justified paragraphs in place that production moves. The
// as-read figure is still logged beside it, and so is the remedy 797 proposed (a justified paragraph broken at its
// flush edge from the start), by producer, justified paragraphs apart — the remedy is adopted only if it is better
// for every producer, and the log is what says.
func TestHowOftenARealProducersParagraphRebreaksInPlace(t *testing.T) {
	corp := externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers"))
	if corp.absent != "" {
		t.Skip("SKIP (not a measurement): " + corp.absent)
	}
	type count struct{ asRead, production, edgeFirst, of int }
	type tally struct{ all, justified count }
	causes := map[string]int{}
	by := map[string]*tally{}
	var producers []string
	var total tally
	for _, doc := range corp.docs {
		ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
		if err != nil {
			continue
		}
		producer := strings.SplitN(doc.name, "/", 2)[0]
		if by[producer] == nil {
			by[producer] = &tally{}
			producers = append(producers, producer)
		}
		for p := 1; p <= ctx.PageCount; p++ {
			l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
			if err != nil {
				causes["unreadable page"]++
				continue
			}
			for pi, para := range l.paragraphs {
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
				asRead := strings.Join(lineTexts(rebreak(words, lineMeasures(lines, spacerOf(lines, space)), spacerOf(lines, space))), "\n") == strings.Join(lineTexts(lines), "\n")
				got, want, justified, _ := rebreakAsProduction(l, pi, false)
				production := got != nil && strings.Join(got, "\n") == strings.Join(want, "\n")
				got, want, _, _ = rebreakAsProduction(l, pi, true)
				edgeFirst := got != nil && strings.Join(got, "\n") == strings.Join(want, "\n")
				for _, c := range []*count{&by[producer].all, &total.all, &by[producer].justified, &total.justified} {
					if (c == &by[producer].justified || c == &total.justified) && !justified {
						continue
					}
					c.of++
					if asRead {
						c.asRead++
					}
					if production {
						c.production++
					}
					if edgeFirst {
						c.edgeFirst++
					}
				}
			}
		}
	}
	line := func(name string, c count) {
		if c.of == 0 {
			return
		}
		t.Logf("%-28s %5d paragraphs: in place %5d as production breaks them, %5d as read (the old census), %5d broken at the flush edge first (797's remedy)",
			name, c.of, c.production, c.asRead, c.edgeFirst)
	}
	for _, p := range producers {
		line(p+", multi-line", by[p].all)
		line(p+", justified", by[p].justified)
	}
	line("real producers, multi-line", total.all)
	line("real producers, justified", total.justified)
	t.Logf("real producers, multi-line paragraphs: %d re-break in place, %d do not, refused by cause %v",
		total.all.production, total.all.of-total.all.production, causes)
	if total.justified.of == 0 {
		t.Errorf("no justified paragraph was read — the path production takes for one was never measured")
	}
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
	id := runMatrix{1, 0, 0, 1, 0, 0}
	return textRun{x: x, width: w, size: 10, decoded: true, widthSrc: widthFromWidths, codes: len(g), glyphs: g,
		state: runTextState{tm: id, ctm: id, scale: 1, th: 1}}
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
		{"text-clips", func(r *textRun) { r.state.tr = 5 }}, // asked here, so the dialog refuses it before anything is typed
	} {
		r := base()
		c.edit(&r)
		if _, _, cause := paragraphWords(textParagraph{lines: []textLine{{runs: []textRun{r}}}}); cause != c.want {
			t.Errorf("want %q, got %q", c.want, cause)
		}
	}
	// A second line drawn STRETCHED by its text matrix is refused here too, before anything is typed (P08.S01's review);
	// one at a uniform scale is restated as a size (P08.S06) and read.
	stretched := base()
	stretched.state.tm = runMatrix{1.5, 0, 0, 1, 0, 0}
	if _, _, cause := paragraphWords(textParagraph{lines: []textLine{{runs: []textRun{base(), glyphRun(5, glyphsOf(" c", 1, nil))}}, {runs: []textRun{stretched}}}}); cause != causeMixedState {
		t.Errorf("a line stretched by its text matrix: want %q, got %q", causeMixedState, cause)
	}
	scaled := base()
	scaled.state.tm = runMatrix{1.5, 0, 0, 1.5, 0, 0}
	if _, _, cause := paragraphWords(textParagraph{lines: []textLine{{runs: []textRun{base(), glyphRun(5, glyphsOf(" c", 1, nil))}}, {runs: []textRun{scaled}}}}); cause != "" {
		t.Errorf("a line at a uniform scale: refused %q, want it read", cause)
	}
	// And the control: the same run unedited is read, and a paragraph with no gap in it cannot say how wide its space is.
	if _, _, cause := paragraphWords(textParagraph{lines: []textLine{{runs: []textRun{base()}}}}); cause != "no-space-width" {
		t.Errorf("a one-word paragraph has no space to measure; got %q", cause)
	}
}
