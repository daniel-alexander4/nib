package pdfops

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"nib/internal/pdfread"

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
	// running is a running header or footer set apart from the page's one body column (`runningLinesOf`): in a cluster of
	// its own, so no body geometry reads it, and not counted in `pageLayout.columns`.
	running bool
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
	// columns counts the body's columns; a running header or footer keeps a cluster (`textParagraph.column`) of its own
	// and is not counted (`runningLinesOf`).
	columns int
	// unsupported says why the page's layout is outside what this rule can read, or is empty. It is
	// set, not guessed around: exit criterion 3 lets a layout be handled OR reported, never silently
	// mis-ordered.
	unsupported string
	noText      bool
	// loose are the runs no line holds (`joinsALine`) and marks the page's non-text marks — kept only by reflow's reader
	// (`readPageGlyphLayout`), which must know what else is drawn where text will move (P07.S01).
	loose []textRun
	marks []pageMark
	// box is the page's visible box — kept only by reflow's reader, so a paragraph can be read as centred on the page
	// (P08.S04).
	box [4]float64
	// sequences are the page's marked-content sequences that carry an MCID (`pageRuns.sequences`) — kept only by reflow's
	// reader, so a carry can take a paragraph's structure to another page with it (P07.S07).
	sequences []markedSeq
	// annots are the boxes of the page's annotations, a popup's excepted (`pageAnchors` without what only moving text asks
	// about) — kept only by reflow's reader, so a re-set line's measure stops short of a field or a note beside it as it
	// stops short of text and drawings (`paragraphMeasures`, P07 phase-close review).
	annots [][4]float64
}

// readPageLayout is the door consumers call: a page's runs, grouped.
// shared is a loop's one budget for the pages it reads, as `readPageRuns` takes it.
func readPageLayout(ctx *model.Context, pg pdfread.Page, shared ...*formWalkBudget) (pageLayout, error) {
	pr, err := readPageRuns(ctx, pg, shared...)
	if err != nil {
		return pageLayout{}, err
	}
	if pr.noText {
		return pageLayout{noText: true}, nil
	}
	return groupRuns(pr.runs), nil
}

// readPageGlyphLayout is readPageLayout over runs that keep their glyphs (`readPageGlyphRuns`) — reflow's reader. The
// grouping is the same door; only what each run carries differs.
func readPageGlyphLayout(ctx *model.Context, pg pdfread.Page) (pageLayout, error) {
	pr, err := readPageGlyphRuns(ctx, pg)
	if err != nil {
		return pageLayout{}, err
	}
	if pr.noText {
		return pageLayout{noText: true, marks: pr.marks, sequences: pr.sequences}, nil
	}
	l := groupRuns(pr.runs)
	l.sequences = pr.sequences
	for _, r := range pr.runs {
		if !joinsALine(r) {
			l.loose = append(l.loose, r)
		}
	}
	l.marks = pr.marks
	l.box = visibleBoxOf(pg)
	for _, a := range pageAnchors(ctx, pg, false) {
		l.annots = append(l.annots, a.box)
	}
	return l, nil
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
	columns, running := runningLinesOf(columnsOf(segments))
	var out pageLayout
	out.columns = len(columns)
	for _, r := range running {
		if r {
			out.columns-- // a running header or footer is not a column (/pending 787)
		}
	}
	// **Rotated text is reported, not read as upright** (`/pending 503`). Every rule below measures a line
	// as a horizontal baseline with x growing rightward. A run whose baseline turns — a vertical margin
	// label, a landscape table on a portrait page — was grouped as if it were upright and nothing said so,
	// so where it sits in reading order and which paragraph it joins were never established. Exit criterion
	// 3 allows a layout to be handled OR reported; this reports, and the page is still proposed.
	if n := rotatedRuns(runs); n > 0 {
		noteUnsupported(&out, fmt.Sprintf("%d run(s) of text are drawn rotated, and the grouping reads every line as upright", n))
	}
	for ci, col := range columns {
		sort.SliceStable(col, func(i, j int) bool { return col[i].y > col[j].y })
		for i := 1; i < len(col); i++ {
			if sameBaseline(col[i-1], col[i]) {
				noteUnsupported(&out, "text sits side by side on one baseline in a way the grouping cannot separate into columns")
			}
		}
		ps := paragraphsOf(col, ci)
		for i := range ps {
			ps[i].running = running != nil && running[ci]
		}
		out.paragraphs = append(out.paragraphs, ps...)
	}
	return out
}

// noteUnsupported adds a reason the page's layout is outside the rule, keeping any reason already given.
func noteUnsupported(l *pageLayout, why string) {
	switch {
	case l.unsupported == "":
		l.unsupported = why
	case !strings.Contains(l.unsupported, why):
		l.unsupported += "; " + why
	}
}

// rotatedRuns counts the runs grouping would read that are drawn rotated — the same population
// `lineSegments` keeps: not an artifact, not blank.
func rotatedRuns(runs []textRun) int {
	n := 0
	for _, r := range runs {
		if r.rotated && !r.artifact && strings.TrimFunc(r.text, unicode.IsSpace) != "" {
			n++
		}
	}
	return n
}

// lineSegments joins runs into lines: same baseline, and no gap wider than joinGapEm.
func lineSegments(runs []textRun) []textLine {
	var rs []textRun
	for _, r := range runs {
		if joinsALine(r) {
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
				if separatesWords(r.x-l.x1, em) && !strings.HasSuffix(l.text, " ") && !strings.HasPrefix(r.text, " ") {
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

// joinsALine says whether a run belongs in a line. An artifact is text the document says is not content — a watermark, a
// running header — so it joins no line and starts no paragraph; nor does a run that draws only white space.
func joinsALine(r textRun) bool {
	return !r.artifact && strings.TrimFunc(r.text, unicode.IsSpace) != ""
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

// runningLinesMax is the most lines a cluster may have and still be read as a running header or footer.
const runningLinesMax = 2

// runningLinesOf says which of a page's clusters are running lines — a header or footer set apart from ONE body column —
// and orders them as the page reads: headers, the body, footers (/pending 787). It returns cols unchanged and nil when
// nothing qualifies.
//
// A centred "Page N of 2" at y 30, or a "1" at y 60, overlaps no body line horizontally when the body is narrower than the
// page, so `columnsOf` makes it a cluster of its own — and a page counted as several columns never flows (reflow's
// `layout.columns != 1` gate, `nextPageFor`), so a growth that needed the next page refused `page-full` on an ordinary
// contract. Every cluster but the body must be a running line: at most `runningLinesMax` lines, narrower than the body,
// and wholly above the body's first baseline or below its last by at least an em — two columns side by side share
// heights, which is what keeps them apart. It is all or nothing: a page that keeps a second column keeps every count and
// order it had. The body is the one cluster with the most lines; a tie is two bodies, and nothing is a running line.
//
// A running line keeps its OWN cluster and is not counted in `pageLayout.columns`. Folding it into the body's cluster
// was tried and refused: the paragraph rule reads a column's right edge, so a page number past the body's right made
// every body line "stop short" and each became a paragraph; and the column's extent is what a centred axis and a
// justified right edge are read against.
//
// Measured before it shipped, over the generated, real-producer and veraPDF PDF/UA-1 corpora (571 pages): one page's
// count changed — census-p60-280 p5, a "Contents" title set 17pt above and left of a single-column contents list, 2 → 1,
// paragraphs and reading order unchanged.
func runningLinesOf(cols [][]textLine) ([][]textLine, []bool) {
	if len(cols) < 2 {
		return cols, nil
	}
	body, most, tie := -1, 0, false
	for i, c := range cols {
		switch {
		case len(c) > most:
			body, most, tie = i, len(c), false
		case len(c) == most:
			tie = true
		}
	}
	if tie {
		return cols, nil
	}
	extent := func(c []textLine) (x0, x1 float64, top, bottom textLine) {
		x0, x1 = math.Inf(1), math.Inf(-1)
		top, bottom = c[0], c[0]
		for _, l := range c {
			x0, x1 = math.Min(x0, l.x0), math.Max(x1, l.x1)
			if l.y > top.y {
				top = l
			}
			if l.y < bottom.y {
				bottom = l
			}
		}
		return x0, x1, top, bottom
	}
	bx0, bx1, btop, bbot := extent(cols[body])
	var headers, footers [][]textLine
	for i, c := range cols {
		if i == body {
			continue
		}
		x0, x1, top, bottom := extent(c)
		if len(c) > runningLinesMax || x1-x0 >= bx1-bx0 {
			return cols, nil
		}
		size := 0.0
		for _, l := range c {
			size = math.Max(size, l.size)
		}
		switch {
		case top.y <= bbot.y-math.Max(size, bbot.size):
			footers = append(footers, c)
		case bottom.y >= btop.y+math.Max(size, btop.size):
			headers = append(headers, c)
		default:
			return cols, nil
		}
	}
	out := append(append(append([][]textLine(nil), headers...), cols[body]), footers...)
	running := make([]bool, len(out))
	for i := range running {
		running[i] = i != len(headers)
	}
	return out, running
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
