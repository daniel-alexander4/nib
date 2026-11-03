package docmodel

import (
	"strings"
	"testing"
)

func cell(t string, col, row int) Cell {
	return Cell{Paragraphs: [][]Span{{{Text: t}}}, ColSpan: col, RowSpan: row}
}

func TestBoundTableClampsSpansAndKeepsASmallTable(t *testing.T) {
	b := Block{Kind: Table, Page: 2, Rows: [][]Cell{{cell("a", 2000000000, 0), cell("b", 1, 99999)}}}
	out, asText := BoundTable(b)
	// 256 + 1 columns passes MaxTableColumns, so a huge span is NOT a table; a span inside the bound is clamped only.
	if !asText || len(out) != 1 || out[0].Kind != Paragraph {
		t.Fatalf("a 2e9-column span: asText=%v, %+v", asText, out)
	}
	b = Block{Kind: Table, Rows: [][]Cell{{cell("a", 3, 400), cell("b", 1, 1)}}}
	out, asText = BoundTable(b)
	if asText || len(out) != 1 || out[0].Rows[0][0].RowSpan != MaxSpan || out[0].Rows[0][0].ColSpan != 3 {
		t.Errorf("a small table with a long row span: asText=%v, %+v", asText, out)
	}
}

func TestBoundTableWritesAWideOrHugeTableAsOneLinePerRowWithEveryWord(t *testing.T) {
	var wide []Cell
	for i := 0; i < MaxTableColumns+1; i++ {
		wide = append(wide, cell("w", 1, 1))
	}
	out, asText := BoundTable(Block{Kind: Table, Page: 4, Dir: "rtl", Rows: [][]Cell{wide, {cell("x", 1, 1), cell("y", 1, 1)}}})
	if !asText || len(out) != 2 || out[0].Page != 4 || out[0].Dir != "rtl" {
		t.Fatalf("a table one column too wide: asText=%v, %d blocks", asText, len(out))
	}
	var line strings.Builder
	for _, s := range out[1].Spans {
		line.WriteString(s.Text)
	}
	if line.String() != "x | y" {
		t.Errorf("row 2 = %q, want %q", line.String(), "x | y")
	}
	// The cell count binds as well as the width: 63 columns by 800 rows is 50,400 cells.
	var tall [][]Cell
	for r := 0; r < 800; r++ {
		row := make([]Cell, MaxTableColumns)
		for c := range row {
			row[c] = cell("z", 1, 1)
		}
		tall = append(tall, row)
	}
	if _, asText := BoundTable(Block{Kind: Table, Rows: tall}); !asText {
		t.Error("63 x 800 cells passed MaxTableCells")
	}
	if _, asText := BoundTable(Block{Kind: Table, Rows: tall[:700]}); asText {
		t.Error("63 x 700 cells (44,100) was refused")
	}
	// A hostile table: 100 cells at span 256 in row one, then 10,000 one-cell rows. It must not become a table.
	hostile := []Cell{}
	for i := 0; i < 100; i++ {
		hostile = append(hostile, cell("h", MaxSpan, 1))
	}
	rows := [][]Cell{hostile}
	for i := 0; i < 10000; i++ {
		rows = append(rows, []Cell{cell("r", 1, 1)})
	}
	if out, asText := BoundTable(Block{Kind: Table, Rows: rows}); !asText || len(out) != 10001 {
		t.Errorf("the hostile table: asText=%v, %d blocks", asText, len(out))
	}
}

func TestBoundTableSpansThatOverflowTheWidthSumAreStillClamped(t *testing.T) {
	// Two cells of MaxInt columns sum to a negative width when added raw; the clamp is what keeps the sum honest.
	big := int(^uint(0) >> 1)
	out, asText := BoundTable(Block{Kind: Table, Rows: [][]Cell{{cell("a", big, 1), cell("b", big, 1)}}})
	if !asText || len(out) != 1 {
		t.Errorf("an overflowing width passed as a table: asText=%v", asText)
	}
}
