package docmodel

// The bounds a table is read under (P03 phase-close review). A span, a width and a cell count come from the PDF's tag
// tree, which a stranger wrote; a writer pads every row to the widest one, so a handful of cells claiming 256 columns
// each and ten thousand one-cell rows is hundreds of millions of cells in a Builder. The bound lives HERE, once, so no
// writer is trusted to have its own (ADR-009).
const (
	// MaxSpan caps a cell's column and row span.
	MaxSpan = 256
	// MaxTableColumns is the widest table written as a table. Word itself will not open one wider than 63.
	MaxTableColumns = 63
	// MaxTableCells is rows times the widest row. A table past it is written as text, one line per row.
	MaxTableCells = 50000
)

// BoundTable returns the table with its spans clamped, or — when its width or its cell count passes a bound — the same
// words as one paragraph per row, cells joined by " | ", and true. No word is dropped either way; only the structure.
func BoundTable(b Block) (out []Block, asText bool) {
	width := 0
	rows := make([][]Cell, len(b.Rows))
	for i, r := range b.Rows {
		rows[i] = make([]Cell, len(r))
		n := 0
		for j, c := range r {
			c.ColSpan = min(max(c.ColSpan, 1), MaxSpan)
			c.RowSpan = min(max(c.RowSpan, 1), MaxSpan)
			rows[i][j] = c
			n += c.ColSpan
		}
		width = max(width, n)
	}
	if width <= MaxTableColumns && len(rows)*width <= MaxTableCells {
		b.Rows = rows
		return []Block{b}, false
	}
	for _, r := range rows {
		var sp []Span
		for j, c := range r {
			if j > 0 {
				sp = append(sp, Span{Text: " | "})
			}
			for k, p := range c.Paragraphs {
				if k > 0 {
					sp = append(sp, Span{Text: " "})
				}
				sp = append(sp, p...)
			}
		}
		out = append(out, Block{Kind: Paragraph, Page: b.Page, Dir: b.Dir, Spans: sp})
	}
	return out, true
}
