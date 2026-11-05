package mdpdf

import (
	"errors"

	east "github.com/yuin/goldmark/extension/ast"
)

const (
	tableCellPad = 4.0  // points inside a cell, each side
	maxTableCols = 24   // a wider table is refused: a column narrower than a word is not a table
	maxTableRows = 5000 // bounds the layout work a stranger's file can ask for
	ruleWeight   = 0.75 // a table's rules, as a thematic break's
)

// ErrTableTooWide, ErrTableTooLong and ErrTableRowTooTall are tables the layout refuses rather than draw wrongly.
var (
	ErrTableTooWide    = errors.New("a table has more columns than the page can set")
	ErrTableTooLong    = errors.New("a table has more rows than one document may carry")
	ErrTableRowTooTall = errors.New("a table row is taller than a page: shorten its cell text")
)

// tableCell is one cell with its words, alignment and, once laid out, its wrapped lines.
type tableCell struct {
	words []word
	align east.Alignment
	lines [][]word
	imgs  []cellImage // pictures under the text, when Options.ImageDir asked for them
}

// table lays out a GFM table: columns sized to their text, cells wrapped inside them, a rule round every cell, the
// header row bold. A row never splits across pages (it moves whole to the next), so each cell's lines stay beside the
// others'. Every cell is its own block with a Role{Table, Row, Head}, in draw order, which is what lets the tagger
// build Table > TR > TH/TD without this package writing a content stream (PLAN-accessibility P06).
func (r *renderer) table(t *east.Table, indent float64) {
	if r.err != nil {
		return
	}
	var rows [][]*tableCell
	var heads []bool
	for row := t.FirstChild(); row != nil && !r.l.stopped(); row = row.NextSibling() {
		_, isHead := row.(*east.TableHeader)
		if len(rows) >= maxTableRows {
			r.err = ErrTableTooLong
			return
		}
		var cells []*tableCell
		for c := row.FirstChild(); c != nil; c = c.NextSibling() {
			cell, ok := c.(*east.TableCell)
			if !ok {
				continue
			}
			words := r.inlineStyled(cell, 0, isHead)
			cells = append(cells, &tableCell{words: words, align: cell.Alignment, imgs: r.cellImages(cell)})
			if r.err != nil {
				return
			}
		}
		heads = append(heads, isHead) // goldmark gives the header's cells as direct children of the TableHeader node
		rows = append(rows, cells)
	}
	cols := 0
	for _, row := range rows {
		cols = max(cols, len(row))
	}
	if cols == 0 {
		return
	}
	if cols > maxTableCols {
		r.err = ErrTableTooWide
		return
	}
	widths := r.columnWidths(rows, cols, r.l.g.contentW()-indent)
	tableW := 0.0
	for _, w := range widths {
		tableW += w
	}
	r.tables++
	num := r.tables
	body := r.f.sty(r.f.body, sizeBody)
	lead := body.leading()
	left := r.l.g.mx + indent

	// Every cell is wrapped and every row measured BEFORE any is drawn: a header repeated on a continuation page needs
	// its height before the body row that forced the break is placed.
	avail := r.l.g.top() - r.l.g.my
	rowH := make([]float64, len(rows))
	for ri, row := range rows {
		inner := 0.0 // the tallest cell: its text, then each picture with a gap above it
		for ci := 0; ci < cols; ci++ {
			c := cellAt(row, ci)
			c.lines = wrapWords(c.words, widths[ci]-2*tableCellPad)
			cellH := float64(max(len(c.lines), 1)) * lead
			for i := range c.imgs {
				c.imgs[i].fitCell(widths[ci]-2*tableCellPad, avail/2)
				cellH += imageGap + c.imgs[i].h
			}
			inner = max(inner, cellH)
		}
		rowH[ri] = inner + 2*tableCellPad
		if rowH[ri] > avail {
			r.err = ErrTableRowTooTall
			return
		}
	}

	// drawRow draws row ri at the cursor.
	drawRow := func(ri int) {
		row, h := rows[ri], rowH[ri]
		y0 := r.l.y
		pg := len(r.l.boxes) - 1
		if ri == 0 || r.l.y == r.l.g.top() {
			r.l.boxes[pg] = append(r.l.boxes[pg], box{left, y0 - ruleWeight/2, tableW, ruleWeight})
		}
		r.l.boxes[pg] = append(r.l.boxes[pg], box{left, y0 - h - ruleWeight/2, tableW, ruleWeight})
		x := left
		for ci := 0; ci < cols; ci++ {
			r.l.boxes[pg] = append(r.l.boxes[pg], box{x - ruleWeight/2, y0 - h, ruleWeight, h})
			c := cellAt(row, ci)
			prev := r.l.beginBlock(Role{Kind: RoleTableCell, Table: num, Row: ri, Head: heads[ri]})
			blk := r.l.blocks
			if len(c.lines) == 0 {
				// An empty cell still has to be a cell: a row with fewer cells than its neighbours is not a
				// regular table, and an element needs a run to be bracketed.
				r.l.y = y0 - tableCellPad - lead
				r.l.add(run{x: x + tableCellPad, y: r.l.y, text: " ", sty: body})
			}
			for li, ln := range c.lines {
				r.l.y = y0 - tableCellPad - lead*float64(li+1)
				r.l.line(ln, x+tableCellPad+alignOffset(ln, c.align, widths[ci]-2*tableCellPad))
			}
			// Pictures sit under the cell's text, each in a figure of its own that names this cell (Figure.Cell).
			top := y0 - tableCellPad - float64(max(len(c.lines), 1))*lead
			for _, im := range c.imgs {
				top -= imageGap + im.h
				r.l.figs[pg] = append(r.l.figs[pg], figure{x: x + tableCellPad, y: top, w: im.w, h: im.h, path: im.path, alt: im.alt, before: len(r.l.runs[pg]), cell: blk})
			}
			r.l.role = prev
			x += widths[ci]
		}
		r.l.boxes[pg] = append(r.l.boxes[pg], box{x - ruleWeight/2, y0 - h, ruleWeight, h})
		r.l.y = y0 - h
	}

	// The header row(s) are the leading rows goldmark marked as the header. They repeat at the top of every page the
	// table continues onto, when that page can hold them and the row that forced the break. The repeat is a header
	// row again (TH, Scope Column) and not an artifact: the tagger writes one Table element per page, so a
	// continuation page's Table would otherwise have no header cells at all (ADR-210).
	headRows, headH := 0, 0.0
	for headRows < len(rows) && heads[headRows] {
		headH += rowH[headRows]
		headRows++
	}
	r.l.gap(paraGap)
	for ri := range rows {
		h := rowH[ri]
		if headRows > 0 && ri >= headRows && r.l.y-h < r.l.g.my && r.l.y < r.l.g.top() && headH+h <= avail {
			r.l.newPage()
			for hi := 0; hi < headRows; hi++ {
				drawRow(hi)
			}
		}
		r.l.need(h)
		drawRow(ri)
	}
	r.l.gap(paraGap)
}

// cellAt is row's ci-th cell, an empty one where the row is short.
func cellAt(row []*tableCell, ci int) *tableCell {
	if ci < len(row) {
		return row[ci]
	}
	return &tableCell{}
}

// alignOffset is how far right a line sits inside a cell of the given text width for the cell's alignment.
func alignOffset(ln []word, a east.Alignment, avail float64) float64 {
	if a != east.AlignRight && a != east.AlignCenter {
		return 0
	}
	w := 0.0
	for i, wd := range ln {
		if i > 0 && len(wd.frags) > 0 {
			w += wd.frags[0].sty.width(" ")
		}
		w += wd.width()
	}
	if w >= avail {
		return 0
	}
	if a == east.AlignRight {
		return avail - w
	}
	return (avail - w) / 2
}

// columnWidths gives each column its natural width when the table fits, and otherwise shares the page between the
// columns by how much each can still give up: every column keeps its widest word, and the room left over goes to the
// ones whose text is longest.
func (r *renderer) columnWidths(rows [][]*tableCell, cols int, avail float64) []float64 {
	minW, maxW := make([]float64, cols), make([]float64, cols)
	for _, row := range rows {
		for ci, c := range row {
			longest, total := 0.0, 0.0
			for i, wd := range c.words {
				w := wd.width()
				longest = max(longest, w)
				total += w
				if i > 0 && len(wd.frags) > 0 {
					total += wd.frags[0].sty.width(" ")
				}
			}
			minW[ci] = max(minW[ci], longest+2*tableCellPad)
			maxW[ci] = max(maxW[ci], total+2*tableCellPad)
		}
	}
	for ci := range minW {
		minW[ci] = max(minW[ci], 3*tableCellPad)
		maxW[ci] = max(maxW[ci], minW[ci])
	}
	sumMin, sumMax := 0.0, 0.0
	for ci := range minW {
		sumMin += minW[ci]
		sumMax += maxW[ci]
	}
	out := make([]float64, cols)
	switch {
	case sumMax <= avail:
		copy(out, maxW)
	case sumMin >= avail:
		for ci := range out { // the page cannot hold every word whole: share it by the words' widths, and the long ones split
			out[ci] = avail * minW[ci] / sumMin
		}
	default:
		spare := avail - sumMin
		for ci := range out {
			out[ci] = minW[ci] + spare*(maxW[ci]-minW[ci])/(sumMax-sumMin)
		}
	}
	return out
}
