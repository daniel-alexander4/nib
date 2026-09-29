package pdfops

import (
	"math"
	"sort"

	"nib/mdpdf"
)

// Reflow — `PLAN-text-reflow.md` P06. A paragraph is re-broken in its own font, at its own measure, by the one line
// breaker (`mdpdf.BreakGreedy`, law 4), with widths the document itself carries (D2).

// reflowWord is one word of a paragraph: its glyphs, and its width as drawn — every glyph's advance and every kern
// between them, but not the kern before its first glyph, which belongs to the gap in front of it.
type reflowWord struct {
	glyphs []runGlyph
	width  float64
	face   *runFont
	size   float64
}

func (w reflowWord) text() string {
	var s string
	for _, g := range w.glyphs {
		s += g.text
	}
	return s
}

// wordGapEm is the smallest gap between two runs on a line, in ems of the later run, that separates two words. Below it
// the runs are one word drawn in pieces (a kerned producer, a change of style inside a word).
const wordGapEm = 0.15

// paragraphWords cuts a paragraph read with its glyphs into its words, line by line, and measures the paragraph's space.
//
// A word ends at a space glyph, at a gap between runs wider than wordGapEm, and at the end of a line. The paragraph's
// space is the MEDIAN of the gaps it actually draws between words — a space glyph's own kern and advance, or the gap
// between runs — because a re-break joins words from different lines, and the only honest width for that join is the
// one this paragraph already uses. The cause is non-empty when the paragraph cannot be reflowed, and names why (law 3).
func paragraphWords(p textParagraph) (lines [][]reflowWord, space float64, cause string) {
	var gaps []float64
	for _, l := range p.lines {
		var line []reflowWord
		var cur *reflowWord
		end := func() {
			if cur != nil && len(cur.glyphs) > 0 {
				line = append(line, *cur)
			}
			cur = nil
		}
		for ri, r := range l.runs {
			switch {
			case r.inForm:
				return nil, 0, "text-in-form"
			case r.rotated:
				return nil, 0, "rotated"
			case r.widthSrc == widthNone:
				return nil, 0, "no-widths"
			case !r.decoded:
				return nil, 0, "undecoded"
			case len(r.glyphs) != r.codes:
				return nil, 0, "glyphs-not-kept"
			}
			if ri > 0 {
				prev := l.runs[ri-1]
				gap := r.x - (prev.x + prev.width)
				if gap > wordGapEm*r.size {
					end()
					gaps = append(gaps, gap)
				}
			}
			for _, g := range r.glyphs {
				if g.text == " " {
					end()
					gaps = append(gaps, g.kern+g.advance)
					continue
				}
				if cur == nil {
					cur = &reflowWord{face: r.face, size: r.size}
					cur.glyphs = append(cur.glyphs, g)
					cur.width += g.advance
					continue
				}
				cur.glyphs = append(cur.glyphs, g)
				cur.width += g.kern + g.advance
			}
		}
		end()
		lines = append(lines, line)
	}
	if len(gaps) == 0 {
		return lines, 0, "no-space-width"
	}
	sort.Float64s(gaps)
	return lines, gaps[len(gaps)/2], ""
}

// measureOf is the widest of the paragraph's lines set with its own space: the measure its breaker used can have been
// no narrower, and a line that would have fitted a wider one was already broken by it.
func measureOf(lines [][]reflowWord, space float64) float64 {
	var widest float64
	for _, l := range lines {
		var w float64
		for i, word := range l {
			if i > 0 {
				w += space
			}
			w += word.width
		}
		widest = math.Max(widest, w)
	}
	return widest
}

// measureSlack absorbs the arithmetic in comparing a sum against the maximum of the same sums.
const measureSlack = 1e-6

// rebreak breaks words at measure with space between them, through the one line breaker. It does not split a word
// wider than the measure; the caller finds it alone on its line.
func rebreak(words []reflowWord, measure, space float64) [][]reflowWord {
	return mdpdf.BreakGreedy(words, measure+measureSlack, mdpdf.BreakOps[reflowWord]{
		Width: func(w reflowWord) float64 { return w.width },
		Space: func(reflowWord) float64 { return space },
	})
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
