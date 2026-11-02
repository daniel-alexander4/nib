package mdpdf

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const sampleTable = "| Item | Qty | Price |\n|:-----|:---:|------:|\n| Pens | 12 | 3.50 |\n| A much longer description of the thing | 1 | 120.00 |\n"

// TestAGFMTableIsLaidOutInCellsWithEachCellItsOwnBlock — I2. A pipe table used to be a paragraph of literal bars.
func TestAGFMTableIsLaidOutInCellsWithEachCellItsOwnBlock(t *testing.T) {
	text, l, err := convertOpts(t, sampleTable, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "|") {
		t.Fatalf("the bars are still in the page text:\n%s", text)
	}
	for _, want := range []string{"Item", "Qty", "Price", "Pens", "12", "3.50", "120.00"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q is not on the page:\n%s", want, text)
		}
	}
	// Every run of a cell is a RoleTableCell with this table's ordinal; rows are numbered from 0, the header row is Head.
	blocks := map[int]Role{}
	for _, r := range l.runs[0] {
		if r.role.Kind != RoleTableCell {
			continue
		}
		if r.role.Table != 1 {
			t.Errorf("run %q has table %d, want 1", r.text, r.role.Table)
		}
		blocks[r.role.Block] = r.role
	}
	if len(blocks) != 9 {
		t.Fatalf("%d cell blocks, want 3 rows x 3 columns = 9", len(blocks))
	}
	heads, bodies := 0, 0
	for _, r := range blocks {
		if r.Head {
			heads++
			if r.Row != 0 {
				t.Errorf("a header cell in row %d", r.Row)
			}
		} else {
			bodies++
		}
	}
	if heads != 3 || bodies != 6 {
		t.Errorf("%d header and %d body cells, want 3 and 6", heads, bodies)
	}
	// The header is bold, the body is not.
	for _, r := range l.runs[0] {
		if r.role.Kind == RoleTableCell && r.role.Head != (r.sty.font == l.f.bold) {
			t.Errorf("run %q: head=%v but font %q (bold is %q)", r.text, r.role.Head, r.sty.font, l.f.bold)
		}
	}
	// Alignment: the right-aligned Price column ends at the same x in every row; the left one starts at the same x.
	var ends []float64
	for _, r := range l.runs[0] {
		if r.role.Kind == RoleTableCell && (r.text == "Price" || r.text == "3.50" || r.text == "120.00") {
			ends = append(ends, r.x+r.sty.width(r.text))
		}
	}
	if len(ends) != 3 || abs(ends[0]-ends[1]) > 0.01 || abs(ends[1]-ends[2]) > 0.01 {
		t.Errorf("a right-aligned column does not end together: %v", ends)
	}
	// Rules: a box round the table (at least a top, a bottom per row, and a vertical per column edge).
	if n := len(l.boxes[0]); n < 1+3+4 {
		t.Errorf("%d rules drawn, want at least a top, one per row and a vertical per column edge", n)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestATableTooWideOrTooLongIsRefused(t *testing.T) {
	var head, sep, row []string
	for i := 0; i < maxTableCols+1; i++ {
		head = append(head, fmt.Sprintf("c%d", i))
		sep = append(sep, "---")
		row = append(row, "x")
	}
	md := "|" + strings.Join(head, "|") + "|\n|" + strings.Join(sep, "|") + "|\n|" + strings.Join(row, "|") + "|\n"
	if _, _, err := convertOpts(t, md, Options{}); !errors.Is(err, ErrTableTooWide) {
		t.Errorf("a %d-column table: %v, want ErrTableTooWide", maxTableCols+1, err)
	}
	var b strings.Builder
	b.WriteString("| a |\n|---|\n")
	for i := 0; i < maxTableRows+1; i++ {
		b.WriteString("| x |\n")
	}
	if _, _, err := convertOpts(t, b.String(), Options{}); !errors.Is(err, ErrTableTooLong) {
		t.Errorf("a very long table: %v, want ErrTableTooLong", err)
	}
}

func TestATableWithAnEmptyCellKeepsItsShapeAndSplitsAcrossPages(t *testing.T) {
	var b strings.Builder
	b.WriteString("| a | b |\n|---|---|\n| x |  |\n")
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&b, "| row%d | v%d |\n", i, i)
	}
	_, l, err := convertOpts(t, b.String(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.runs) < 2 {
		t.Fatalf("%d page(s): a table of 82 rows should continue onto another", len(l.runs))
	}
	// Row 1 has an empty second cell: it must still have two blocks (the second a lone space), so the row is regular.
	cells := map[int]map[int]bool{}
	for _, page := range l.runs {
		for _, r := range page {
			if r.role.Kind == RoleTableCell && r.role.Row == 1 {
				if cells[1] == nil {
					cells[1] = map[int]bool{}
				}
				cells[1][r.role.Block] = true
			}
		}
	}
	if len(cells[1]) != 2 {
		t.Errorf("row 1 has %d cell blocks, want 2 even with an empty cell", len(cells[1]))
	}
	// Placeholders work inside a cell.
	got, _, err := convertOpts(t, "| k |\n|---|\n| {{v}} |\n", Options{Vars: map[string]string{"v": "FILLED"}})
	if err != nil || !strings.Contains(got, "FILLED") {
		t.Errorf("a placeholder in a cell: %q %v", got, err)
	}
}
