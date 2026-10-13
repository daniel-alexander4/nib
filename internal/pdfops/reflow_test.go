package pdfops

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/mdpdf"
)

// rebreak breaks words at the paragraph's line measures with space between them, through the one line breaker — the
// breaker as the rewrite calls it, over the words the paragraph already has. A test instrument.
func rebreak(words []reflowWord, measures []float64, space wordSpacer) [][]reflowWord {
	return breakAt(words, measures, func(w reflowWord) float64 { return w.width },
		func(w reflowWord) float64 { return space(w.face, w.tfSize, lastSpacing(w.spacing)) }, nil, nil, nil)
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
	got, want, justified, _, cause = breakAsProduction(l, pi, nil, edgeFirst)
	return got, want, justified, cause
}

// breakAsProduction is `rebreakAsProduction` for an EDITED paragraph too (/pending 797 W2): readback is the edit's
// lines as the written document reads them, and they are broken with what production took from paragraph pi as it
// stood BEFORE the edit — its alignment, its one word spacing, its spacer, its measures, and the lines the edit left
// alone (`keptLines`, by the alignment of the read-back words to the paragraph's) — which is what `reflowParagraphIn`
// breaks an edit's words with. want is then the read-back's own lines. nil breaks the paragraph's own words, by the
// breaker ALONE: with nothing changed every line is kept and the answer would be the paragraph as read whatever the
// breaker does, so this is what a line the edit changed, and those after it, are given. The hooks of a hyphenated
// break are still not modelled: a break the rewrite kept reads back as two words and one it rejoined as one, and each
// is broken as the word it now is.
func breakAsProduction(l pageLayout, pi int, readback [][]reflowWord, edgeFirst bool) (got, want []string, justified, centred bool, cause string) {
	var kept []int
	if readback != nil {
		lines, _, cause := paragraphWords(l.paragraphs[pi])
		if cause != "" {
			return nil, nil, false, false, cause
		}
		var old, edited []string
		for _, ln := range lines {
			for _, w := range ln {
				old = append(old, w.text())
			}
		}
		for _, ln := range readback {
			for _, w := range ln {
				edited = append(edited, w.text())
			}
		}
		kept = make([]int, len(edited))
		for ni, oi := range alignWords(old, edited).kept {
			kept[ni] = oi + 1
		}
	}
	return modelBreak(l, pi, readback, kept, edgeFirst)
}

// breakWithout breaks paragraph pi's own words with word at (counted from 0 across its lines) taken away — or, retyped,
// left where it is as a word the edit typed, kept as no occurrence — as production breaks that edit, every other word
// kept as the occurrence it is; keep false leaves `keptLines` out, which is the rewrite as it was before /pending 797.
// want is the paragraph's lines as read, less the word taken away: what the edit leaves when it moves no other word.
func breakWithout(l pageLayout, pi, at int, retyped, keep bool) (got, want []string, cause string) {
	lines, _, cause := paragraphWords(l.paragraphs[pi])
	if cause != "" {
		return nil, nil, cause
	}
	var from [][]reflowWord
	kept := []int{}
	n := 0
	for _, ln := range lines {
		var row []reflowWord
		for _, w := range ln {
			switch {
			case n != at:
				row, kept = append(row, w), append(kept, n+1)
			case retyped:
				row, kept = append(row, w), append(kept, 0)
			}
			n++
		}
		if len(row) > 0 {
			from = append(from, row)
		}
	}
	if !keep {
		kept = nil
	}
	got, want, _, _, cause = modelBreak(l, pi, from, kept, false)
	return got, want, cause
}

// modelBreak is the instrument behind the two: readback are the words to break, in the lines they should come out in
// (nil: paragraph pi's own), and kept names the paragraph word each one keeps, 1 + its index (0: none; a nil kept: no
// line is kept at all).
func modelBreak(l pageLayout, pi int, readback [][]reflowWord, kept []int, edgeFirst bool) (got, want []string, justified, centred bool, cause string) {
	lines, gap, cause := paragraphWords(l.paragraphs[pi])
	if cause != "" {
		return nil, nil, false, false, cause
	}
	want = lineTexts(lines)
	if readback != nil {
		want = lineTexts(readback)
	}
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
		if readback != nil {
			readback = withWordSpacing(readback, tw)
		}
	}
	space := spacerOf(lines, gap)
	var words []emitWord
	var texts []string
	from := lines
	if readback != nil {
		from = readback
	}
	for _, ln := range from {
		for _, w := range ln {
			ew := emitWord{face: w.face, tfSize: w.tfSize, spacing: w.spacing, width: w.width, seq: len(words) + 1}
			if len(words) < len(kept) {
				ew.oldIdx = kept[len(words)]
			}
			words = append(words, ew)
			texts = append(texts, w.text())
		}
	}
	var keepLine func(int, []emitWord, emitWord) mdpdf.LineKeep
	if kept != nil {
		var oldAt [][2]int
		for li, ln := range lines {
			for wi := range ln {
				oldAt = append(oldAt, [2]int{li, wi})
			}
		}
		keepLine = keptLines(words, lines, oldAt, space)
	}
	width := func(w emitWord) float64 { return w.width }
	atEdge := func() [][]emitWord {
		edge := make([]float64, len(lines))
		for i, ln := range lines {
			edge[i] = align.edge - ln[0].startX
		}
		return breakAt(words, edge, width,
			func(w emitWord) float64 { return space(w.face, w.tfSize, lastSpacing(w.spacing)) + align.lastDelta }, nil, nil, keepLine)
	}
	var broken [][]emitWord
	if edgeFirst && align.justified {
		broken = atEdge()
		if _, fits := justifiedLines(align, broken, lines, space); !fits {
			return nil, want, true, false, causeWordTooWide
		}
	} else {
		measures := paragraphMeasures(l, pi, lines, space)
		if align.centred {
			for i := range measures {
				measures[i] = align.width
			}
		}
		broken = breakAt(words, measures, width, func(w emitWord) float64 { return space(w.face, w.tfSize, lastSpacing(w.spacing)) }, nil, nil, keepLine)
		if _, fits := justifiedLines(align, broken, lines, space); !fits {
			broken = atEdge()
			if _, fits = justifiedLines(align, broken, lines, space); !fits {
				return nil, want, align.justified, align.centred, causeWordTooWide
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
	return got, want, align.justified, align.centred, ""
}

// resetLines rewrites paragraph pi of page p of pdf as edit, writes the document, and returns the lines the written
// page reads at the baselines the rewrite set — the paragraph's own, then one pitch down for each line it grew by.
// cause is the rewrite's refusal, and then there are no lines. For the corpus censuses' typed and grown edits
// (/pending 797 I5).
func resetLines(t *testing.T, pdf []byte, l pageLayout, p, pi int, edit string) (got []textLine, cause string) {
	t.Helper()
	para := l.paragraphs[pi]
	c2, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	out, err := reflowParagraph(c2, p, pi, edit)
	if err != nil {
		t.Fatalf("p%d ¶%d: %v", p, pi, err)
	}
	if out.content == nil {
		return nil, out.cause
	}
	d, _, _, _ := c2.PageDict(p, false)
	if err := setPageContent(c2, d, out.content); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := api.WriteContext(c2, &buf); err != nil {
		t.Fatal(err)
	}
	c3, err := pdfread.Validated(buf.Bytes(), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("p%d ¶%d: the rewritten document does not read: %v", p, pi, err)
	}
	l3, _ := readPageGlyphLayout(c3, pageAt(c3, nil, p))
	pitch := paragraphPitch(l, pi)
	for i, n := 0, 0; n < editLetters(edit); i++ {
		y := para.lines[min(i, len(para.lines)-1)].y - float64(max(0, i-len(para.lines)+1))*pitch
		ln, ok := lineAtBaseline(l3, paragraphBox(para), y, para.lines[0].size, para.lines[min(i, len(para.lines)-1)].x0)
		if !ok {
			break
		}
		got = append(got, ln)
		n += editLetters(ln.text)
	}
	return got, ""
}

// backwards is word spelled backwards, and whether that is a NEW word for a paragraph reading text: longer than two
// letters, not a palindrome, and not a word the paragraph already draws — so the rewrite must TYPE it.
func backwards(word, text string) (string, bool) {
	rs := []rune(word)
	for a, b := 0, len(rs)-1; a < b; a, b = a+1, b-1 {
		rs[a], rs[b] = rs[b], rs[a]
	}
	rev := string(rs)
	return rev, len(rs) > 2 && rev != word && !strings.Contains(" "+text+" ", " "+rev+" ")
}

// TestHowOftenARealProducersParagraphRebreaksInPlace is a MEASUREMENT of the breaker: over the real-producer corpus, how
// many multi-line paragraphs it re-breaks where they were broken when nothing in them changes. Skips without the corpus.
//
// **It is no longer what keeps an edit on one line from moving a word on another** (/pending 797): the breaker cannot
// reproduce every producer's breaks, so the rewrite keeps the breaks an edit has no bearing on (`keptLines`), and this
// census says how often that matters — every paragraph not in place here is one where, before, an edit moved words on
// lines it never touched. What an edit now moves is measured beside it, through the same instrument with the lines kept
// (`breakWithout`), and that IS asserted: with a paragraph's last word taken away no word of another line moves; with
// one word retyped — on its first line, where its middle line begins, at its end — keeping lines never moves more
// paragraphs than the breaker alone, and moves no more than `retypedMovesMost` (the paragraphs left are justified ones
// whose natural space is read from a last line that is itself set wide; they are re-broken narrow, as before).
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
	// An edit of one word, through the instrument with the lines kept and with the breaker alone: how many paragraphs it
	// was made in, and in how many it moved no other word.
	type edited struct{ of, kept, alone int }
	edits := map[string]*edited{}
	editNames := []string{"its last word taken away", "its first word retyped", "the first word of its middle line retyped", "its last word retyped"}
	for _, n := range editNames {
		edits[n] = &edited{}
	}
	var movedThough []string
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
				n, mid := len(words), 0
				for _, ln := range lines[:len(lines)/2] {
					mid += len(ln)
				}
				for name, e := range map[string]struct {
					at      int
					retyped bool
					skip    bool
				}{
					editNames[0]: {at: n - 1, skip: len(lines[len(lines)-1]) < 2}, // a last line of one word would go with it
					editNames[1]: {at: 0, retyped: true},
					editNames[2]: {at: mid, retyped: true},
					editNames[3]: {at: n - 1, retyped: true},
				} {
					if e.skip {
						continue
					}
					edits[name].of++
					for _, keep := range []bool{true, false} {
						got, want, cause := breakWithout(l, pi, e.at, e.retyped, keep)
						switch same := cause == "" && got != nil && strings.Join(got, "\n") == strings.Join(want, "\n"); {
						case same && keep:
							edits[name].kept++
						case same:
							edits[name].alone++
						case keep:
							movedThough = append(movedThough, fmt.Sprintf("%s p%d, with %s (%s): %q", doc.name, p, name, cause, got))
						}
					}
				}
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
	for _, name := range editNames {
		e := edits[name]
		t.Logf("with %-42s %5d paragraphs: no other word moves in %5d with the lines kept, %5d by the breaker alone", name+",", e.of, e.kept, e.alone)
		if e.kept < e.alone {
			t.Errorf("with %s, keeping lines moves a word in more paragraphs (%d of %d unmoved) than the breaker alone (%d)", name, e.kept, e.of, e.alone)
		}
		if e.of-e.kept > retypedMovesMost {
			t.Errorf("with %s, a word of another line moves in %d of %d paragraphs; at most %d are known to", name, e.of-e.kept, e.of, retypedMovesMost)
		}
	}
	if e := edits[editNames[0]]; e.kept != e.of || e.of < total.all.of/2 {
		t.Errorf("with a paragraph's last word taken away, a word of another line moved in %d of %d paragraphs (of %d multi-line): none may, and half must be asked", e.of-e.kept, e.of, total.all.of)
	}
	for _, m := range movedThough {
		t.Logf("moved though its lines were kept: %s", m)
	}
}

// retypedMovesMost is the most paragraphs of the real-producer corpus in which one word retyped still moves a word of
// another line — measured 3, 2 and 1 for the three retyped words (65 each by the breaker alone): `acrobat/fda-99140.pdf`
// p30 (its last line runs past the edge its lines above share) and three of `antennahouse/irs-p15.pdf` (p10, p32, p53:
// a last line set wide, from which the natural space is read).
const retypedMovesMost = 3

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
