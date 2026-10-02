package pdfops

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/mdpdf"
)

// TestASizeSetByTheTextMatrixIsRestatedAsAFontSize — `PLAN-text-reflow.md` P08.S06: InDesign sets `Tf 1` and the size
// in `Tm`; a paragraph whose lines mix that with `Tf 12` under an unscaled matrix was refused `mixed-state`. Each glyph is
// now restated at the paragraph's first run's scale — its size, and the letter spacing of the unscaled line (0.5 at
// scale 1 is 0.5/12 at the first run's 12) — so the paragraph is re-set: every word reads back at its own user size and
// width, and the line after the paragraph is drawn where, and as, it was.
//
// **The stimulus is asserted**: the paragraph's runs are drawn at two text-matrix scales.
func TestASizeSetByTheTextMatrixIsRestatedAsAFontSize(t *testing.T) {
	pdf := helveticaPage("BT /F1 1 Tf 12 0 0 12 72 700 Tm (The quick brown fox jumps) Tj" +
		" /F1 12 Tf 0.5 Tc 1 0 0 1 72 686 Tm (over the lazy dog and runs) Tj" +
		" /F1 1 Tf 0 Tc 12 0 0 12 72 672 Tm (away from here.) Tj" +
		" /F1 12 Tf 1 0 0 1 72 600 Tm (Signature line) Tj ET")
	before, runs := layoutOf(t, pdf)
	scales := map[float64]bool{}
	for _, r := range runs {
		scales[r.state.scale] = true
	}
	if len(scales) < 2 || len(before.paragraphs) < 2 {
		t.Fatalf("setup: %d scales, %d paragraphs — want two scales and the signature apart", len(scales), len(before.paragraphs))
	}
	widths := map[string]float64{}
	bl, _, cause := paragraphWords(before.paragraphs[0])
	if cause != "" {
		t.Fatalf("refused (%s), want it read", cause)
	}
	for _, l := range bl {
		for _, w := range l {
			widths[w.text()] = w.width
		}
	}
	edit := "The quick brown fox leaps over the lazy dog and runs away from here."
	out, cause := reflowed(t, pdf, 0, edit)
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	after, afterRuns := layoutOf(t, out)
	al, _, cause := paragraphWords(after.paragraphs[0])
	if cause != "" {
		t.Fatalf("the rewritten paragraph cannot be read (%s)", cause)
	}
	var got []string
	for _, l := range al {
		for _, w := range l {
			got = append(got, w.text())
			if want, ok := widths[w.text()]; ok && math.Abs(w.width-want) > 1e-6 {
				t.Errorf("%q reads %v wide, it was %v", w.text(), w.width, want)
			}
		}
	}
	if strings.Join(got, " ") != edit {
		t.Errorf("reads back %q", strings.Join(got, " "))
	}
	for _, r := range afterRuns {
		if math.Abs(r.size-12) > 1e-6 {
			t.Errorf("a run %q reads at size %v, want 12", r.text, r.size)
		}
		if r.text == "Signature line" && (math.Abs(r.x-72) > 1e-6 || math.Abs(r.y-600) > 1e-6) {
			t.Errorf("the signature line moved to %v,%v", r.x, r.y)
		}
	}
}

// TestAFigureRaisedByItsTextMatrixStaysRaised — a footnote figure drawn in another font, smaller and raised by its own
// text matrix, abutting its word ("smoke¹³"), was `styled-word` and `mixed-state` both. Re-set, the figure is still in its
// font, at its size and 3 points above its word's baseline (grouping joins a run within 0.3 em of the line) — wherever the edit moves the word.
//
// **The stimulus is asserted**: the figure is one word with "smoke", in another font, size and baseline.
func TestAFigureRaisedByItsTextMatrixStaysRaised(t *testing.T) {
	x := 72 + mdpdf.CoreWidth("Here is all the smoke", "Helvetica", 12)
	content := fmt.Sprintf("BT /F1 12 Tf 1 0 0 1 72 700 Tm (Here is all the smoke) Tj /F2 1 Tf 7 0 0 7 %s 703 Tm (13) Tj"+
		" /F1 12 Tf 1 0 0 1 %s 700 Tm ( that the fire makes in) Tj 1 0 0 1 72 686 Tm (one long night and day.) Tj ET",
		num(x), num(x+mdpdf.CoreWidth("13", "Helvetica-Bold", 7)))
	pdf := twoFontPage("F1", "F2", content)
	before, _ := layoutOf(t, pdf)
	bl, _, cause := paragraphWords(before.paragraphs[0])
	if cause != "" {
		t.Fatalf("refused (%s), want it read", cause)
	}
	if w := findWord(bl, "smoke13"); w == nil || w.looks[5].font != "F2" {
		t.Fatalf("setup: no word smoke13 in a second font in %v", lineTexts(bl))
	}
	edit := "Here is all the smoke13 that the fire makes in one long night and day."
	out, cause := reflowed(t, pdf, 0, strings.Replace(edit, "Here is all", "All", 1))
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	_, runs := layoutOf(t, out)
	var base, fig *textRun
	for i := range runs {
		r := &runs[i]
		if strings.HasSuffix(r.text, "smoke") || strings.Contains(r.text, "smoke") {
			base = r
		}
		if strings.TrimSpace(r.text) == "13" {
			fig = r
		}
	}
	if base == nil || fig == nil {
		t.Fatalf("runs %v: no smoke and 13", runs)
	}
	if fig.font != "F2" || math.Abs(fig.size-7) > 1e-6 || math.Abs(fig.y-(base.y+3)) > 1e-6 {
		t.Errorf("the figure reads in %s at %v, %v above its word — want F2 at 7, 3 above", fig.font, fig.size, fig.y-base.y)
	}
	// A second copy of the word, typed, is that word — drawn once, in one look of two fonts — and is not ambiguous.
	if _, cause := reflowed(t, pdf, 0, edit+" smoke13"); cause != "" {
		t.Errorf("a typed copy of the two-font word refused (%s)", cause)
	}
}

func findWord(lines [][]reflowWord, text string) *reflowWord {
	for _, l := range lines {
		for i := range l {
			if l[i].text() == text {
				return &l[i]
			}
		}
	}
	return nil
}

// TestStylesOverTheCorpus — S06 over the real-producer corpus: every readable paragraph that holds a word in two fonts,
// or runs at another text-matrix scale (both refused before S06 — `styled-word`, `mixed-state`), is given a one-word
// edit (its last word removed), re-set where it can be, and read back: every word it kept has, glyph by glyph, the font
// and the size in user space it was drawn in. What still refuses is counted by cause.
func TestStylesOverTheCorpus(t *testing.T) {
	corp := externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers"))
	if corp.absent != "" {
		t.Skip("SKIP (not a measurement): " + corp.absent)
	}
	population, reset, checked := 0, 0, 0
	still := map[string]int{}
	// A glyph's font, its size in user space (the restated Tf size at the reference scale) and its height above its
	// line's baseline (the rise, in user space) — every occurrence of a word, not the first alone (the review, W4, I2).
	type glyphKey struct {
		font       string
		size, rise float64
	}
	lookOf := func(ws [][]reflowWord) map[string][]string {
		out := map[string][]string{}
		for _, l := range ws {
			for _, w := range l {
				var k []glyphKey
				for gi, lk := range w.looks {
					k = append(k, glyphKey{lk.font, math.Round(lk.tfSize*w.scale*1000) / 1000, math.Round(w.spacing[gi].ts*w.scale*100) / 100})
				}
				out[w.text()] = append(out[w.text()], fmt.Sprint(k))
			}
		}
		return out
	}
	for _, doc := range corp.docs {
		ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
		if err != nil {
			continue
		}
		for p := 1; p <= ctx.PageCount; p++ {
			l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
			if err != nil {
				continue
			}
			for pi, para := range l.paragraphs {
				lines, _, cause := paragraphWords(para)
				if len(editWords(para.text())) < 3 {
					continue
				}
				scaled, styled := false, false
				for _, ln := range para.lines {
					for _, r := range ln.runs {
						scaled = scaled || math.Abs(r.state.scale-para.lines[0].runs[0].state.scale) > 1e-9
					}
				}
				for _, ln := range lines {
					for _, w := range ln {
						for _, lk := range w.looks {
							styled = styled || lk.font != w.looks[0].font
						}
					}
				}
				if !scaled && !styled {
					continue
				}
				population++
				if cause != "" {
					still["read:"+cause]++
					continue
				}
				ws := editWords(para.text())
				edit := strings.Join(ws[:len(ws)-1], " ")
				c2, _ := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
				out, err := reflowParagraph(c2, p, pi, edit)
				if err != nil {
					t.Fatalf("%s p%d ¶%d: %v", doc.name, p, pi, err)
				}
				if out.content == nil {
					still[out.cause]++
					continue
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
					t.Fatalf("%s p%d ¶%d: the rewritten document does not read: %v", doc.name, p, pi, err)
				}
				l3, _ := readPageGlyphLayout(c3, pageAt(c3, nil, p))
				reset++
				var got []textLine
				for i, n := 0, 0; i < len(para.lines) && n < editLetters(edit); i++ {
					ln, ok := lineAtBaseline(l3, paragraphBox(para), para.lines[i].y, para.lines[0].size)
					if !ok {
						break
					}
					got = append(got, ln)
					n += editLetters(ln.text)
				}
				gl, _, gcause := paragraphWords(textParagraph{lines: got, column: para.column})
				if gcause != "" {
					t.Errorf("%s p%d ¶%d: the re-set paragraph cannot be read (%s)", doc.name, p, pi, gcause)
					continue
				}
				want, have := lookOf(lines), lookOf(gl)
				for text, ks := range have {
					w, ok := want[text]
					if !ok {
						continue
					}
					for _, k := range ks {
						checked++
						if !slices.Contains(w, k) {
							t.Errorf("%s p%d ¶%d: %q reads %s, it was drawn %v", doc.name, p, pi, text, k, w)
						}
					}
				}
			}
		}
	}
	t.Logf("paragraphs with a word in two fonts or runs at two scales: %d; re-set %d (%d words checked glyph by glyph); still refused %v",
		population, reset, checked, still)
	if reset == 0 || checked == 0 {
		t.Errorf("re-set %d, checked %d — the census asked nothing", reset, checked)
	}
}

// TestASlantedLineKeepsItsBaseline — the review of S06, C1: under a slanted text matrix (a synthetic italic, x sheared by
// y), a later run on the line is offset ALONG the line, and read as a projection onto the slanted up axis that offset
// became a rise — "brown fox" re-set 14 points high. Solved on the matrix's own axes it is no rise at all.
//
// **The stimulus is asserted**: the line's second run starts away from its first, under a sheared matrix.
func TestASlantedLineKeepsItsBaseline(t *testing.T) {
	x := 72 + 12*mdpdf.CoreWidth("The quick ", "Helvetica", 1)
	pdf := helveticaPage(fmt.Sprintf("BT /F1 1 Tf 12 0 2.4 12 72 700 Tm (The quick ) Tj 12 0 2.4 12 %s 700 Tm (brown fox jumps over) Tj"+
		" 12 0 2.4 12 72 686 Tm (the lazy dog and runs.) Tj ET", num(x)))
	_, runs := layoutOf(t, pdf)
	if len(runs) < 3 || runs[1].state.tm[2] == 0 || math.Abs(runs[1].x-runs[0].x) < 10 {
		t.Fatalf("setup: runs %d, the second not sheared or not apart", len(runs))
	}
	out, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog and runs.")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	_, after := layoutOf(t, out)
	// Both runs of the first line are ON it — the line's base run is "brown fox …", so an error lands on "The quick".
	seen := 0
	for _, r := range after {
		if strings.Contains(r.text, "The") || strings.Contains(r.text, "brown") {
			seen++
			if math.Abs(r.y-700) > 1e-6 {
				t.Errorf("%q re-set at y %v, want 700", r.text, r.y)
			}
		}
	}
	if seen == 0 {
		t.Errorf("no first-line words read back")
	}
}

// TestJitterOffTheBaselineIsNotARise — the review of S06, I1: producers draw runs a fraction of a point off their line's
// baseline (89 runs 0.01–1 pt in the corpus). Carried as rise, each would make a word unlike its other copies and keep the
// jitter wherever it moved; under a tenth of an em it is the line's baseline.
func TestJitterOffTheBaselineIsNotARise(t *testing.T) {
	x := 72 + mdpdf.CoreWidth("The quick brown ", "Helvetica", 12)
	pdf := helveticaPage(fmt.Sprintf("BT /F1 12 Tf 1 0 0 1 72 700 Tm (The quick brown ) Tj 1 0 0 1 %s 700.5 Tm (fox jumps over the) Tj"+
		" 1 0 0 1 72 686 Tm (lazy dog and runs away.) Tj ET", num(x)))
	_, runs := layoutOf(t, pdf)
	if len(runs) < 2 || math.Abs(runs[1].y-700.5) > 1e-6 {
		t.Fatalf("setup: the second run is not 0.5 off the line")
	}
	out, cause := reflowed(t, pdf, 0, "The quick brown fox leaps over the lazy dog and runs away.")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	_, after := layoutOf(t, out)
	for _, r := range after {
		if math.Abs(r.y-700) < 3.6 && math.Abs(r.y-700) > 1e-6 && math.Abs(r.y-700.5) > 1e-6 {
			t.Errorf("%q re-set at y %v", r.text, r.y)
		}
		if math.Abs(r.y-700.5) < 1e-6 && strings.Contains(r.text, "fox") {
			// The base run decides the line: whichever run is the jitter, the re-set line has ONE baseline.
			for _, q := range after {
				if math.Abs(q.y-700) < 1e-6 {
					t.Errorf("the re-set line has two baselines: %q at 700 and %q at 700.5", q.text, r.text)
				}
			}
		}
	}
}

// footnotePage is a footnote: its first line opens with its figure in F2, 7 points and raised 3, then two body lines.
func footnotePage() []byte {
	return twoFontPage("F1", "F2", "BT /F2 1 Tf 7 0 0 7 72 703 Tm (1) Tj /F1 12 Tf 1 0 0 1 76 700 Tm (The quick brown fox jumps over the) Tj"+
		" 1 0 0 1 72 686 Tm (lazy dog and runs away from the) Tj 1 0 0 1 72 672 Tm (barn.) Tj ET")
}

// TestALineOpeningWithARaisedFigureIsSetOnItsBody — the review of S06, C2 and W1: a footnote's first line opens with
// its raised figure. Measured from the line's FIRST run, every body word on it read as lowered by 3 points, and kept that
// offset wherever the edit moved it; and a typed word took the figure's font, size and height. The line's baseline is its
// base run's: a moved word sits on its new line's baseline, a typed word is in the body's look, the figure stays raised.
//
// **The stimulus is asserted**: the line's first run is the figure, in F2, raised.
func TestALineOpeningWithARaisedFigureIsSetOnItsBody(t *testing.T) {
	pdf := footnotePage()
	_, runs := layoutOf(t, pdf)
	if runs[0].font != "F2" || math.Abs(runs[0].y-703) > 1e-6 {
		t.Fatalf("setup: the first run is %q in %s at %v, want the figure in F2 at 703", runs[0].text, runs[0].font, runs[0].y)
	}
	out, cause := reflowed(t, pdf, 0, "1 The quick brown fox jumps over the very lazy dog and runs away from the barn.")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	_, after := layoutOf(t, out)
	for _, r := range after {
		switch {
		case strings.TrimSpace(r.text) == "1":
			if r.font != "F2" || math.Abs(r.size-7) > 1e-6 || math.Abs(r.y-703) > 1e-6 {
				t.Errorf("the figure reads in %s at %v, y %v — want F2, 7, 703", r.font, r.size, r.y)
			}
		default:
			if r.font != "F1" || math.Abs(r.size-12) > 1e-6 {
				t.Errorf("%q reads in %s at %v, want the body's F1 at 12", r.text, r.font, r.size)
			}
			if ok := math.Abs(r.y-700) < 1e-6 || math.Abs(r.y-686) < 1e-6 || math.Abs(r.y-672) < 1e-6 || math.Abs(r.y-658) < 1e-6; !ok {
				t.Errorf("%q reads at y %v, off every line's baseline", r.text, r.y)
			}
		}
	}
}

// TestTheSpaceAfterATrailingFigureIsTheBodys — the review of S06, W2: the space after "smoke¹³" was drawn in the
// figure's font at 7 points, 42% narrow. It is the body's: re-set, the gap after the figure is the space the paragraph
// draws between its body words.
func TestTheSpaceAfterATrailingFigureIsTheBodys(t *testing.T) {
	x := 72 + mdpdf.CoreWidth("Here is all the smoke", "Helvetica", 12)
	content := fmt.Sprintf("BT /F1 12 Tf 1 0 0 1 72 700 Tm (Here is all the smoke) Tj /F2 1 Tf 7 0 0 7 %s 703 Tm (13) Tj"+
		" /F1 12 Tf 1 0 0 1 %s 700 Tm ( that the fire makes in) Tj 1 0 0 1 72 686 Tm (one long night and day.) Tj ET",
		num(x), num(x+mdpdf.CoreWidth("13", "Helvetica-Bold", 7)))
	out, cause := reflowed(t, twoFontPage("F1", "F2", content), 0, "All the smoke13 that the fire makes in one long night and day.")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	l, _ := layoutOf(t, out)
	lines, _, _ := paragraphWords(l.paragraphs[0])
	var gaps []float64
	var after float64 = -1
	for _, ln := range lines {
		for i := 1; i < len(ln); i++ {
			g := ln[i].startX - ln[i-1].startX - ln[i-1].width
			if ln[i-1].text() == "smoke13" {
				after = g
			} else {
				gaps = append(gaps, g)
			}
		}
	}
	if after < 0 || len(gaps) == 0 || math.Abs(after-gaps[0]) > 1e-6 {
		t.Errorf("the space after the figure is %v, the body's %v", after, gaps)
	}
}

// TestAWordInTwoFontsIsAmbiguousOnlyByItsFonts — the review of S06, W3: "Word" drawn once in F1 and once in F2 looks two
// ways, so a typed third copy cannot say which it is — `ambiguous-style` — while a paragraph drawing it twice in F1 lends
// the copy freely. The font half of sameLook decides it.
func TestAWordInTwoFontsIsAmbiguousOnlyByItsFonts(t *testing.T) {
	two := twoFontPage("F1", "F2", "BT /F1 12 Tf 14 TL 72 700 Td (The Word and the ) Tj /F2 12 Tf (Word) Tj /F1 12 Tf ( again are) Tj T* (here today.) Tj ET")
	if _, cause := reflowed(t, two, 0, "The Word and the Word again are here today Word"); cause != causeAmbiguousStyle {
		t.Errorf("a typed copy of a word drawn in two fonts: cause %q, want %q", cause, causeAmbiguousStyle)
	}
	one := twoFontPage("F1", "F2", "BT /F1 12 Tf 14 TL 72 700 Td (The Word and the Word again are) Tj T* (here today.) Tj ET")
	if _, cause := reflowed(t, one, 0, "The Word and the Word again are here today Word"); cause != "" {
		t.Errorf("a typed copy of a word drawn in one font refused (%s)", cause)
	}
}

// TestAFigureMovedToAnotherLineStaysRaised — the review of S06, W4: the raised figure test never moved its word to
// another line. Here the edit carries "smoke¹³" from the first line to the second: the figure is 3 points above the
// baseline of the line it now sits on.
func TestAFigureMovedToAnotherLineStaysRaised(t *testing.T) {
	x := 72 + mdpdf.CoreWidth("Here is all the smoke", "Helvetica", 12)
	content := fmt.Sprintf("BT /F1 12 Tf 1 0 0 1 72 700 Tm (Here is all the smoke) Tj /F2 1 Tf 7 0 0 7 %s 703 Tm (13) Tj"+
		" /F1 12 Tf 1 0 0 1 %s 700 Tm ( that the fire makes in) Tj 1 0 0 1 72 686 Tm (one long night and day.) Tj ET",
		num(x), num(x+mdpdf.CoreWidth("13", "Helvetica-Bold", 7)))
	out, cause := reflowed(t, twoFontPage("F1", "F2", content), 0,
		"Early in the cold grey morning, before the sun came up over the far hills, here is all of the thick black smoke13 that the fire makes in one long night and day.")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	_, runs := layoutOf(t, out)
	var fig *textRun
	for i := range runs {
		if strings.TrimSpace(runs[i].text) == "13" {
			fig = &runs[i]
		}
	}
	if fig == nil || fig.y > 699 {
		t.Fatalf("setup: the figure did not move off the first line (%v)", fig)
	}
	for _, r := range runs {
		if r.font == "F1" && math.Abs(r.y-fig.y) < 3.6 {
			if math.Abs(fig.y-r.y-3) > 1e-6 {
				t.Errorf("the figure reads %v above its new line's baseline %v, want 3", fig.y-r.y, r.y)
			}
			return
		}
	}
	t.Errorf("no body text on the figure's line")
}
