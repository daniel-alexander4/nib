package pdfops

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Lines and paragraphs — `PLAN-accessibility.md` P08.S03, which is `PLAN-text-reflow.md` P04.
//
// **The rule exists here and nowhere else** (reflow D10, ADR-009): the autotagger and reflow both need
// to know where a paragraph begins, and two implementations would be two different answers in one
// product. Consumers call `readPageLayout`; a guard asserts nothing outside this file and `textrun.go`
// touches runs. `groupWords` (`tagocr.go`) is not a second opinion — it takes tesseract's own grouping
// and infers nothing.
//
// # What was measured before the rule was written
//
// On the generated corpus (P08.S03's step zero):
//
//   - **Runs on one baseline abut exactly.** nib's Markdown draws a styled line as several runs and each
//     starts where the previous ends (176.17 → 176.17), so adjacency joins a line.
//   - **Two columns share baselines.** LibreOffice's two-column section puts both columns' lines on
//     identical y values, 36pt apart horizontally — a rule joining by baseline alone fuses them. So a
//     gap wider than 1.5 em splits a baseline into segments; the Markdown bullet sits 1.3 em from its
//     text and stays joined.
//   - **Paragraph spacing is not reliable.** mdpdf adds 6pt after a paragraph (step 20.85 against
//     14.85); LibreOffice's default paragraph adds nothing (13.8 against 13.8). Vertical gap is one
//     signal among four, not the rule.
//   - **A short line is not a paragraph end by itself.** Ragged-right text leaves 24.6pt at the end of a
//     line mid-paragraph because the next word did not fit. The break signal is that the next line's
//     first word WOULD have fit.

// textLine is runs sharing a baseline and adjacent enough to be one line, left to right.
type textLine struct {
	runs   []textRun
	x0, x1 float64
	y      float64
	size   float64 // the largest run size on the line
	text   string
}

// textParagraph is consecutive lines in one column that the rule does not break.
type textParagraph struct {
	lines  []textLine
	column int
}

func (p textParagraph) text() string {
	parts := make([]string, len(p.lines))
	for i, l := range p.lines {
		parts[i] = l.text
	}
	return strings.Join(parts, " ")
}

// pageLayout is a page grouped into paragraphs in reading order.
type pageLayout struct {
	paragraphs []textParagraph
	columns    int
	// unsupported says why the page's layout is outside what this rule can read, or is empty. It is
	// set, not guessed around: exit criterion 3 lets a layout be handled OR reported, never silently
	// mis-ordered.
	unsupported string
	noText      bool
}

// readPageLayout is the door consumers call: a page's runs, grouped.
func readPageLayout(ctx *model.Context, pageNr int) (pageLayout, error) {
	pr, err := readPageRuns(ctx, pageNr)
	if err != nil {
		return pageLayout{}, err
	}
	if pr.noText {
		return pageLayout{noText: true}, nil
	}
	return groupRuns(pr.runs), nil
}

// joinGapEm is the widest horizontal gap, in ems of the larger run, that still joins two runs on a
// baseline into one line.
const joinGapEm = 1.5

// groupRuns groups one page's runs into paragraphs in reading order.
func groupRuns(runs []textRun) pageLayout {
	segments := lineSegments(runs)
	if len(segments) == 0 {
		return pageLayout{noText: true}
	}
	columns := columnsOf(segments)
	var out pageLayout
	out.columns = len(columns)
	for ci, col := range columns {
		sort.SliceStable(col, func(i, j int) bool { return col[i].y > col[j].y })
		for i := 1; i < len(col); i++ {
			if sameBaseline(col[i-1], col[i]) {
				out.unsupported = "text sits side by side on one baseline in a way the grouping cannot separate into columns"
			}
		}
		out.paragraphs = append(out.paragraphs, paragraphsOf(col, ci)...)
	}
	return out
}

// lineSegments joins runs into lines: same baseline, and no gap wider than joinGapEm.
func lineSegments(runs []textRun) []textLine {
	var rs []textRun
	for _, r := range runs {
		// An artifact is text the document says is not content — a watermark, a running header — so it
		// joins no line and starts no paragraph.
		if !r.artifact && strings.TrimFunc(r.text, unicode.IsSpace) != "" {
			rs = append(rs, r)
		}
	}
	sort.SliceStable(rs, func(i, j int) bool {
		if !approxBaseline(rs[i], rs[j]) {
			return rs[i].y > rs[j].y
		}
		return rs[i].x < rs[j].x
	})
	var out []textLine
	for _, r := range rs {
		if n := len(out); n > 0 {
			l := &out[n-1]
			em := math.Max(l.size, r.size)
			if math.Abs(l.y-r.y) <= 0.3*em && r.x-l.x1 <= joinGapEm*em {
				if r.x-l.x1 > 0.15*em && !strings.HasSuffix(l.text, " ") && !strings.HasPrefix(r.text, " ") {
					l.text += " "
				}
				l.text += r.text
				l.runs = append(l.runs, r)
				l.x1 = math.Max(l.x1, r.x+r.width)
				l.size = em
				continue
			}
		}
		out = append(out, textLine{runs: []textRun{r}, x0: r.x, x1: r.x + r.width, y: r.y, size: r.size, text: r.text})
	}
	for i := range out {
		out[i].text = strings.TrimSpace(out[i].text)
	}
	return out
}

func approxBaseline(a, b textRun) bool {
	return math.Abs(a.y-b.y) <= 0.3*math.Max(a.size, b.size)
}

func sameBaseline(a, b textLine) bool {
	return math.Abs(a.y-b.y) <= 0.3*math.Max(a.size, b.size)
}

// columnsOf clusters line segments by overlapping horizontal extent, left to right.
func columnsOf(segments []textLine) [][]textLine {
	sorted := append([]textLine(nil), segments...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].x0 < sorted[j].x0 })
	type col struct {
		x0, x1 float64
		lines  []textLine
	}
	var cols []*col
	for _, s := range sorted {
		placed := false
		for _, c := range cols {
			if s.x0 < c.x1 && s.x1 > c.x0 {
				c.lines = append(c.lines, s)
				c.x0, c.x1 = math.Min(c.x0, s.x0), math.Max(c.x1, s.x1)
				placed = true
				break
			}
		}
		if !placed {
			cols = append(cols, &col{x0: s.x0, x1: s.x1, lines: []textLine{s}})
		}
	}
	out := make([][]textLine, len(cols))
	for i, c := range cols {
		out[i] = c.lines
	}
	return out
}

// paragraphsOf breaks one column's lines (top to bottom) into paragraphs.
func paragraphsOf(lines []textLine, column int) []textParagraph {
	if len(lines) == 0 {
		return nil
	}
	right := 0.0
	var steps []float64
	for i, l := range lines {
		right = math.Max(right, l.x1)
		if i > 0 {
			steps = append(steps, lines[i-1].y-l.y)
		}
	}
	typical := median(steps)

	var out []textParagraph
	cur := textParagraph{column: column, lines: []textLine{lines[0]}}
	for i := 1; i < len(lines); i++ {
		prev, l := lines[i-1], lines[i]
		if breaksBefore(prev, l, right, typical, len(steps)) {
			out = append(out, cur)
			cur = textParagraph{column: column}
		}
		cur.lines = append(cur.lines, l)
	}
	return append(out, cur)
}

// breaksBefore is the paragraph rule: any one of four signals starts a new paragraph at l.
func breaksBefore(prev, l textLine, right, typical float64, steps int) bool {
	em := math.Max(prev.size, l.size)
	// A size change: a heading and its body are not one paragraph.
	if math.Abs(prev.size-l.size) > 0.1*em {
		return true
	}
	// A vertical step clearly wider than the column's usual one. With a single step there is no usual.
	if steps >= 2 && prev.y-l.y > typical*1.25 {
		return true
	}
	// An indent: a line starting further right than the one above begins something new.
	if l.x0 > prev.x0+0.5*em {
		return true
	}
	// A line that stopped short when the next line's first word would have fit.
	return right-prev.x1 > firstWordWidth(l)+0.5*em
}

// firstWordWidth estimates the width of a line's first word from its first run, by the word's share of
// the run's characters. Glyph widths vary, so it is an estimate — and only ever compared against a
// shortfall, where proportional spacing moves the answer by less than the half-em margin allows.
func firstWordWidth(l textLine) float64 {
	r := l.runs[0]
	text := strings.TrimLeftFunc(r.text, unicode.IsSpace)
	n := len([]rune(text))
	if n == 0 {
		return 0
	}
	word := strings.IndexFunc(text, unicode.IsSpace)
	if word < 0 {
		return r.width
	}
	return r.width * float64(len([]rune(text[:word]))) / float64(n)
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}
