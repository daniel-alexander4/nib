package pdfops

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The proposer — `PLAN-accessibility.md` P08.S05.
//
// Law 3 and D5: inferred structure is a PROPOSAL. This file turns a document's paragraphs into
// proposed elements — headings with levels, paragraphs, list items grouped into lists — and writes
// nothing. S06 shows the proposal, lets a person change it, and only a commit writes. A guard asserts
// no function here calls a writer.
//
// # The rules, and what they were cut from
//
// Measured on LibreOffice's own output before a line was written (P08.S05's step zero):
//
//   - **Headings are LARGER.** 18pt and 16pt against a 12pt body, in a bold face. Size is the signal
//     taken; the bold face name is not, because nothing in the corpus needs it and a bold lead-in
//     sentence at body size would be the first thing it got wrong.
//   - **Body size is the size carrying the most characters**, not the most common run: a page of short
//     headings and one long paragraph has more heading runs than body runs.
//   - **A list label is its own token.** LibreOffice draws a bullet as U+F095 in OpenSymbol — a
//     private-use code point, not `•` — and a number as `1.` in the body font, each followed by the
//     item's text. A line whose first token is one of those starts a list item.
//   - **The grouping merges a list that has nothing wider beside it.** Two short numbered items alone
//     in a column are one paragraph to S03, because the column's right edge IS their width and the
//     short-line signal cannot fire. A label at the start of a line splits them here.
//
// # Tables (ADR-121)
//
// A table is proposed where the page RULES one and nowhere else: a regular grid of at least two rows and two
// columns (`ruledGrids`). It is the one proposed element with children — `Table`, a `TR` for each row, a `TH`
// or `TD` for each cell, the first row's cells as headers — so an element names its parent. A cell with no
// text is still an element: the rows stay the same length, which is what makes it a table to a reader.

// proposedElement is one element the autotagger proposes.
type proposedElement struct {
	// role is a standard structure type: H1–H6, P or LI, one of a table's — Table, TR, TH, TD — or Figure.
	role string
	// parent is the index, in the proposal, of the element this one sits under, or -1 at the top level.
	// Only a table has children: a TR's parent is its Table and a cell's its TR, each directly after its
	// parent's last child, so a table's elements are consecutive and in row order (ADR-121).
	parent int
	// row and col are a cell's place in its table, from 0; a TR carries its row.
	row, col int
	// box is the extent of a table, a row or a cell — the grid's own, which an empty cell needs and which
	// outlines a cell where its text alone would not. boxed says it is set.
	box    [4]float64
	boxed  bool
	page   int
	column int
	// marker is a list item's label as drawn (`1.`, U+F095), and text includes it: the label is part
	// of what the item shows, and `LI` holds `Lbl` and `LBody` both.
	marker string
	text   string
	lines  []textLine
	// list numbers the list a list item belongs to — consecutive items share one — or is -1.
	list int
	// figure is the operator that draws a Figure's image — `Do` with its operand, or `BI … EI` — in the page's
	// own content; a Figure has no text, and box is the image as drawn (ADR-122).
	figure opSpan
	// alt is the description a reviewer gave a Figure. The proposer sets none: nothing here can say what a
	// picture shows.
	alt string
}

// tableRoles are the roles of a table's own elements; cellRoles the two a cell may have.
var (
	tableRoles = map[string]bool{"Table": true, "TR": true, "TH": true, "TD": true}
	cellRoles  = map[string]bool{"TH": true, "TD": true}
)

// proposal is a document's proposed structure, in reading order.
type proposal struct {
	elements []proposedElement
	bodySize float64
	// unsupported names pages whose layout the grouping could not read, with its reason: those pages'
	// elements are proposed in the order the grouping gave, and the reviewer is told why to look.
	unsupported map[int]string
	// noText lists pages with no text at all — a scan's path is OCR, not a proposal.
	noText []int
}

// headingScale is how much larger than the body a paragraph must be to be proposed as a heading.
const headingScale = 1.15

// maxHeadingLines bounds a heading's length: a long passage set large is large text, not a title.
const maxHeadingLines = 3

// proposeStructure reads every page through the layout door and proposes a structure for the whole
// document. It reads; it never writes.
func proposeStructure(ctx *model.Context) (proposal, error) {
	layouts := make([]pageLayout, 0, ctx.PageCount)
	budget := newFormWalkBudget(ctx.PageCount) // one for the document (`readPageRuns`)
	for _, pg := range pdfread.Pages(ctx) {
		l, err := readPageTableLayout(ctx, pg, budget)
		if err != nil {
			return proposal{}, err
		}
		layouts = append(layouts, l)
	}
	return proposeFromLayouts(layouts), nil
}

// proposeFromLayouts is the rule, over layouts already read.
func proposeFromLayouts(layouts []pageLayout) proposal {
	out := proposal{unsupported: map[int]string{}}
	out.bodySize = bodySizeOf(layouts)
	levels := headingLevels(layouts, out.bodySize)

	lists, inList := 0, false
	for i, l := range layouts {
		page := i + 1
		if l.noText {
			out.noText = append(out.noText, page)
			continue
		}
		if l.unsupported != "" {
			out.unsupported[page] = l.unsupported
		}
		blocks := pageBlocks(l)
		emit := func(b pageBlock) {
			if b.table == nil {
				out.elements = append(out.elements, proposedElement{role: figureRole, parent: -1, page: page, list: -1,
					box: b.figure.box, boxed: true, figure: b.figure.span})
				return
			}
			out.elements = appendTable(out.elements, page, *b.table)
			inList = false
		}
		for pi, par := range l.paragraphs {
			for len(blocks) > 0 && blocks[0].before <= pi {
				emit(blocks[0])
				blocks = blocks[1:]
			}
			for _, piece := range splitAtListMarkers(par) {
				first := piece.lines[0]
				el := proposedElement{page: page, column: piece.column, text: piece.text(), lines: piece.lines, list: -1, parent: -1}
				switch m := listMarker(first); {
				case m != "":
					if !inList {
						lists++
						inList = true
					}
					el.role, el.marker, el.list = "LI", m, lists-1
				case isHeadingPiece(piece, out.bodySize):
					inList = false
					el.role = fmt.Sprintf("H%d", levels[roundHalf(first.size)])
				default:
					inList = false
					el.role = "P"
				}
				out.elements = append(out.elements, el)
			}
		}
		for _, b := range blocks {
			emit(b)
		}
	}
	return out
}

// figureRole is the role of the one proposed element that is a picture (ADR-122).
const figureRole = "Figure"

// pageBlock is something a page's proposal holds between its paragraphs: a table or a figure.
type pageBlock struct {
	table  *ruledTable
	figure pageFigure
	// before is how many of the page's paragraphs are proposed before it.
	before int
}

// pageBlocks is a page's tables and figures in the order they are proposed.
//
// **The tables keep the order and the places they had before a figure was ever proposed** — a table is proposed
// once every table before it has been, so its place is the furthest any of them reached — and each figure is
// set among them: before the first table that comes after it. Where a figure and a table have the same number
// of paragraphs before them, the one whose top is higher comes first, and of two with one top the one further
// left (ADR-122). A figure ends no list: the items either side of a picture are proposed as the one list they
// were, and a KEPT figure between them parts them at the commit, where lists follow the reviewed order.
func pageBlocks(l pageLayout) []pageBlock {
	blocks := make([]pageBlock, 0, len(l.tables)+len(l.figures))
	reached := 0
	for i := range l.tables {
		reached = max(reached, l.tables[i].before)
		blocks = append(blocks, pageBlock{table: &l.tables[i], before: reached})
	}
	figures := append([]pageFigure(nil), l.figures...)
	sort.SliceStable(figures, func(i, j int) bool {
		a, b := figures[i], figures[j]
		if a.before != b.before {
			return a.before < b.before
		}
		return above(a.box, b.box)
	})
	for _, f := range figures {
		at := len(blocks)
		for j, b := range blocks {
			if b.table != nil && (b.before > f.before || (b.before == f.before && above(f.box, b.table.grid.frame()))) {
				at = j
				break
			}
		}
		blocks = append(blocks, pageBlock{})
		copy(blocks[at+1:], blocks[at:])
		blocks[at] = pageBlock{figure: f, before: f.before}
	}
	return blocks
}

// above says box a is read before box b where nothing else orders them: its top is higher, or it is as high
// and starts further left.
func above(a, b [4]float64) bool {
	if a[3] != b[3] {
		return a[3] > b[3]
	}
	return a[0] < b[0]
}

// appendTable adds a table to a proposal: the Table, then each row's TR and its cells, row by row. The first
// row's cells are proposed as headers — where a ruled table has a header row it is the first, and a reviewer
// who knows better retypes a cell. A Table and a TR draw nothing themselves; their text is their cells',
// joined, so a table that changed after it was proposed no longer matches its review.
func appendTable(elements []proposedElement, page int, t ruledTable) []proposedElement {
	g := t.grid
	table := len(elements)
	elements = append(elements, proposedElement{role: "Table", parent: -1, page: page, list: -1, box: g.frame(), boxed: true})
	var all []string
	for r := 0; r < g.rows(); r++ {
		tr := len(elements)
		elements = append(elements, proposedElement{role: "TR", parent: table, row: r, page: page, list: -1, box: g.rowBox(r), boxed: true})
		var rowText []string
		for c := 0; c < g.cols(); c++ {
			cell := proposedElement{role: "TD", parent: tr, row: r, col: c, page: page, list: -1,
				lines: t.cells[r][c], box: g.cellBox(r, c), boxed: true}
			if r == 0 {
				cell.role = "TH"
			}
			cell.text = textParagraph{lines: cell.lines}.text()
			if cell.text != "" {
				rowText = append(rowText, cell.text)
			}
			elements = append(elements, cell)
		}
		elements[tr].text = strings.Join(rowText, " ")
		all = append(all, rowText...)
	}
	elements[table].text = strings.Join(all, " ")
	return elements
}

// headerScope is the `/Scope` a header cell is written with: a cell of the first row heads its column; any
// other header in the first column heads its row; one anywhere else is read as heading its column.
func headerScope(el proposedElement) string {
	if el.row > 0 && el.col == 0 {
		return "Row"
	}
	return "Column"
}

func roundHalf(x float64) float64 { return math.Round(x*2) / 2 }

// bodySizeOf is the size carrying the most characters across the document.
func bodySizeOf(layouts []pageLayout) float64 {
	chars := map[float64]int{}
	for _, l := range layouts {
		count := func(lines []textLine) {
			for _, ln := range lines {
				for _, r := range ln.runs {
					chars[roundHalf(r.size)] += len([]rune(strings.Join(strings.Fields(r.text), "")))
				}
			}
		}
		for _, par := range l.paragraphs {
			count(par.lines)
		}
		// A table's text counts as it did when it was proposed as paragraphs: a document that is mostly table
		// has the table's size as its body, and its introduction is not a heading for being larger.
		for _, t := range l.tables {
			for _, row := range t.cells {
				for _, cell := range row {
					count(cell)
				}
			}
		}
	}
	best, most := 0.0, -1
	for size, n := range chars {
		if n > most || (n == most && size < best) {
			best, most = size, n
		}
	}
	return best
}

func isHeadingPiece(p textParagraph, body float64) bool {
	return body > 0 && len(p.lines) <= maxHeadingLines && roundHalf(p.lines[0].size) >= body*headingScale
}

// headingLevels numbers the distinct heading sizes, largest first, from 1 to 6.
//
// It reads the PIECES `proposeFromLayouts` classifies, by that function's own tests (/pending 653). Read from
// whole paragraphs, a long paragraph whose first piece was a heading had no level, the lookup answered 0, and
// the role "H0" made `CommitTags` refuse the whole proposal.
func headingLevels(layouts []pageLayout, body float64) map[float64]int {
	seen := map[float64]bool{}
	for _, l := range layouts {
		for _, par := range l.paragraphs {
			for _, piece := range splitAtListMarkers(par) {
				if listMarker(piece.lines[0]) == "" && isHeadingPiece(piece, body) {
					seen[roundHalf(piece.lines[0].size)] = true
				}
			}
		}
	}
	sizes := make([]float64, 0, len(seen))
	for s := range seen {
		sizes = append(sizes, s)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(sizes)))
	out := map[float64]int{}
	for i, s := range sizes {
		out[s] = min(i+1, 6)
	}
	return out
}

// splitAtListMarkers starts a new piece at every line, after the first, that begins with a list label.
func splitAtListMarkers(par textParagraph) []textParagraph {
	var out []textParagraph
	cur := textParagraph{column: par.column}
	for i, ln := range par.lines {
		if i > 0 && listMarker(ln) != "" {
			out = append(out, cur)
			cur = textParagraph{column: par.column}
		}
		cur.lines = append(cur.lines, ln)
	}
	return append(out, cur)
}

// numberedLabel is a list number: `1.`, `12)`, `a.`, `iv)`. Uppercase single letters are excluded —
// `A.` begins ordinary sentences and initials far more often than it labels a list.
var numberedLabel = regexp.MustCompile(`^(\d{1,3}|[a-z]|[ivxlcdm]{1,4})[.)]$`)

// listMarker returns a line's list label, or "". The label must be followed by the item's text.
//
// **The label is read from the first RUN as well as the first word**, because a producer that draws
// the label as its own run need not leave a space before the text: LibreOffice's numbered list draws
// `1.` flush against `Open the document.`, so the line's first word is `1.Open` and matches nothing.
// Found by the truth corpus, where the proposal cleared its score floors and still called all three
// items paragraphs.
func listMarker(ln textLine) string {
	isLabel := func(tok string) bool {
		if numberedLabel.MatchString(tok) {
			return true
		}
		r := []rune(tok)
		return len(r) == 1 && isBulletRune(r[0])
	}
	if len(ln.runs) >= 2 {
		if tok := strings.TrimSpace(ln.runs[0].text); isLabel(tok) {
			return tok
		}
	}
	if fields := strings.Fields(ln.text); len(fields) >= 2 && isLabel(fields[0]) {
		return fields[0]
	}
	return ""
}

// isBulletRune reports the characters documents draw as list bullets — including the private-use
// area, where symbol fonts put theirs (LibreOffice's OpenSymbol bullet is U+F095, measured).
func isBulletRune(r rune) bool {
	switch r {
	case '•', '◦', '▪', '▫', '■', '□', '●', '○', '‣', '⁃', '–', '—', '-', '*', '·', '➢', '✓':
		return true
	}
	return r >= 0xE000 && r <= 0xF8FF && !unicode.IsLetter(r)
}
