package pdfops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The table proposal — ADR-121: a table is proposed from the rules the page draws, regular grids only,
// nested in the proposal.

// ruledFixture is an untagged one-page document drawing content in Helvetica.
func ruledFixture(content string) []byte {
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	})
}

func drawText(x, y float64, s string) string {
	return fmt.Sprintf("BT /F1 12 Tf %g %g Td (%s) Tj ET\n", x, y, s)
}

func hRule(x0, x1, y float64) string { return fmt.Sprintf("%g %g m %g %g l S\n", x0, y, x1, y) }
func vRule(x, y0, y1 float64) string { return fmt.Sprintf("%g %g m %g %g l S\n", x, y0, x, y1) }

// The fixtures' grid: three rows of two columns.
var (
	gridXs = []float64{100, 250, 400}
	gridYs = []float64{700, 670, 640, 610}
)

// ruledLines draws every rule of the grid as a stroked line.
func ruledLines() string {
	var b strings.Builder
	for _, y := range gridYs {
		b.WriteString(hRule(gridXs[0], gridXs[len(gridXs)-1], y))
	}
	for _, x := range gridXs {
		b.WriteString(vRule(x, gridYs[len(gridYs)-1], gridYs[0]))
	}
	return b.String()
}

// cellTexts draws one word in each cell of the grid, row by row; "" leaves a cell empty.
func cellTexts(words ...string) string {
	var b strings.Builder
	for i, w := range words {
		if w == "" {
			continue
		}
		r, c := i/2, i%2
		b.WriteString(drawText(gridXs[c]+6, gridYs[r+1]+10, w))
	}
	return b.String()
}

var sixWords = []string{"Name", "Qty", "Apple", "3", "Pear", "5"}

// proposed is a proposal as role, parent and text of each element — what a table changes.
func proposed(t *testing.T, pdf []byte) (rows []string, pr TagProposal) {
	t.Helper()
	pr, err := ProposeTags(pdf)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range pr.Elements {
		rows = append(rows, fmt.Sprintf("%s^%d %q", e.Role, e.Parent, e.Text))
	}
	return rows, pr
}

func tableNotes(pr TagProposal) []string {
	var out []string
	for _, u := range pr.Unsupported {
		out = append(out, u.Reason)
	}
	return out
}

// TestARuledGridIsProposedAsATable — a regular grid, however its rules are drawn, is one Table holding a TR
// for each row and a cell for each column of it, the first row's as headers; an empty cell is still a cell.
func TestARuledGridIsProposedAsATable(t *testing.T) {
	tiled, slivers := "", ""
	for r := 0; r < 3; r++ {
		for c := 0; c < 2; c++ {
			tiled += fmt.Sprintf("%g %g %g %g re S\n", gridXs[c], gridYs[r+1], gridXs[c+1]-gridXs[c], gridYs[r]-gridYs[r+1])
		}
	}
	// Rules a producer draws as filled slivers, each overlapping its neighbours at the corners.
	for _, y := range gridYs {
		slivers += fmt.Sprintf("%g %g %g 1 re f\n", gridXs[0]-0.5, y-0.5, gridXs[2]-gridXs[0]+1)
	}
	for _, x := range gridXs {
		slivers += fmt.Sprintf("%g %g 1 %g re f\n", x-0.5, gridYs[3]-0.5, gridYs[0]-gridYs[3]+1)
	}
	full := []string{
		`Table^-1 "Name Qty Apple 3 Pear 5"`,
		`TR^0 "Name Qty"`, `TH^1 "Name"`, `TH^1 "Qty"`,
		`TR^0 "Apple 3"`, `TD^4 "Apple"`, `TD^4 "3"`,
		`TR^0 "Pear 5"`, `TD^7 "Pear"`, `TD^7 "5"`,
	}
	for _, c := range []struct {
		name, content string
		want          []string
	}{
		{"stroked lines", ruledLines() + cellTexts(sixWords...), full},
		{"tiled rectangles", tiled + cellTexts(sixWords...), full},
		{"filled slivers", slivers + cellTexts(sixWords...), full},
		// Each rule in two pieces that meet in the middle of a cell's side.
		{"rules drawn in pieces", hRule(100, 180, 700) + hRule(180, 400, 700) + hRule(100, 320, 670) + hRule(320, 400, 670) +
			hRule(100, 180, 640) + hRule(180, 400, 640) + hRule(100, 180, 610) + hRule(180, 400, 610) +
			vRule(100, 610, 655) + vRule(100, 655, 700) + vRule(250, 610, 685) + vRule(250, 685, 700) + vRule(400, 610, 625) + vRule(400, 625, 700) +
			cellTexts(sixWords...), full},
		{"one empty cell", ruledLines() + cellTexts("Name", "Qty", "Apple", "", "Pear", "5"), []string{
			`Table^-1 "Name Qty Apple Pear 5"`,
			`TR^0 "Name Qty"`, `TH^1 "Name"`, `TH^1 "Qty"`,
			`TR^0 "Apple"`, `TD^4 "Apple"`, `TD^4 ""`,
			`TR^0 "Pear 5"`, `TD^7 "Pear"`, `TD^7 "5"`,
		}},
		{"two lines in one cell", ruledLines() + cellTexts(sixWords...) + drawText(106, 696-12, "Given"), []string{
			`Table^-1 "Given Name Qty Apple 3 Pear 5"`,
			`TR^0 "Given Name Qty"`, `TH^1 "Given Name"`, `TH^1 "Qty"`,
			`TR^0 "Apple 3"`, `TD^4 "Apple"`, `TD^4 "3"`,
			`TR^0 "Pear 5"`, `TD^7 "Pear"`, `TD^7 "5"`,
		}},
	} {
		pdf := ruledFixture(c.content)
		got, pr := proposed(t, pdf)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n  got  %v\n  want %v", c.name, got, c.want)
			continue
		}
		if notes := tableNotes(pr); len(notes) > 0 {
			t.Errorf("%s: a table that was read is reported %v", c.name, notes)
		}
		// Every element of a table carries the grid's own box, so an empty cell can be pointed at.
		for i, e := range pr.Elements {
			if e.List != -1 || e.Page != 1 {
				t.Errorf("%s: element %d is on page %d in list %d", c.name, i, e.Page, e.List)
			}
		}
		if r := pr.Elements[0].Rect; r != [4]float64{100, 610, 400, 700} {
			t.Errorf("%s: the table's box is %v", c.name, r)
		}
		if r := pr.Elements[4].Rect; r != [4]float64{100, 640, 400, 670} {
			t.Errorf("%s: the second row's box is %v", c.name, r)
		}
		if r := pr.Elements[6].Rect; r != [4]float64{250, 640, 400, 670} {
			t.Errorf("%s: the second row's second cell's box is %v", c.name, r)
		}
		// CommitTags proposes again from the bytes and requires the review to match: the same bytes must
		// read as the same proposal.
		if again, _ := proposed(t, pdf); !reflect.DeepEqual(again, got) {
			t.Errorf("%s: a second proposal of the same bytes differs:\n  %v\n  %v", c.name, got, again)
		}
	}
}

// TestWhatIsNotARegularRuledGridIsNotATable — every drawing here proposes exactly what the same text proposes
// with nothing drawn. A grid with a merged cell says so on its page; the others say nothing.
func TestWhatIsNotARegularRuledGridIsNotATable(t *testing.T) {
	para := drawText(106, 680, "A paragraph in a box.")
	merged := hRule(100, 400, 700) + hRule(100, 400, 670) + hRule(100, 400, 640) + hRule(100, 400, 610) +
		vRule(100, 610, 700) + vRule(400, 610, 700) + vRule(250, 610, 670) // the first row is one cell
	for _, c := range []struct {
		name, drawn, text string
		note              string
	}{
		{"a box round a paragraph", "100 670 300 30 re S\n", para, ""},
		{"an underlined heading", hRule(100, 300, 676), para, ""},
		{"two parallel rules", hRule(100, 400, 700) + hRule(100, 400, 670), para, ""},
		// The strips share their page with a rule and a stroke that touch nothing, so the page has rules enough
		// for a grid and only the strip's own shape says it is not one.
		{"a strip of one row", hRule(100, 300, 500) + vRule(500, 400, 450) + hRule(100, 400, 700) + hRule(100, 400, 670) + vRule(100, 670, 700) + vRule(250, 670, 700) + vRule(400, 670, 700),
			drawText(106, 680, "Left") + drawText(256, 680, "Right"), ""},
		{"a strip of one column", hRule(100, 300, 500) + vRule(500, 400, 450) + hRule(100, 250, 700) + hRule(100, 250, 670) + hRule(100, 250, 640) + vRule(100, 640, 700) + vRule(250, 640, 700),
			drawText(106, 680, "Upper") + drawText(106, 650, "Lower"), ""},
		{"a grid with a merged cell", merged, cellTexts("Fruit in stock", "", "Apple", "3", "Pear", "5"), mergedCellsNote},
		{"a merged grid holding no text", merged, drawText(100, 580, "Under an empty form."), ""},
		{"a grid with a cell merged down", hRule(100, 400, 700) + hRule(250, 400, 670) + hRule(100, 400, 640) + hRule(100, 400, 610) +
			vRule(100, 610, 700) + vRule(250, 610, 700) + vRule(400, 610, 700), cellTexts(sixWords...), mergedCellsNote},
		// A frame that is not whole is no table and no merged cell: nothing says what it bounds.
		{"a grid with part of its top missing", hRule(100, 250, 700) + hRule(100, 400, 670) + hRule(100, 400, 640) + hRule(100, 400, 610) +
			vRule(100, 610, 700) + vRule(250, 610, 700) + vRule(400, 610, 700), cellTexts(sixWords...), ""},
		{"a grid with part of its left side missing", hRule(100, 400, 700) + hRule(100, 400, 670) + hRule(100, 400, 640) + hRule(100, 400, 610) +
			vRule(100, 610, 670) + vRule(250, 610, 700) + vRule(400, 610, 700), cellTexts(sixWords...), ""},
		{"a grid whose rows have no side", hRule(60, 400, 700) + hRule(60, 400, 670) + hRule(60, 400, 640) +
			vRule(100, 640, 700) + vRule(250, 640, 700) + vRule(400, 640, 700),
			drawText(66, 680, "a") + drawText(106, 680, "b") + drawText(256, 680, "c") + drawText(66, 650, "d") + drawText(106, 650, "e") + drawText(256, 650, "f"), ""},
		{"a grid painted white", "1 G\n" + ruledLines() + "0 G\n", cellTexts(sixWords...), ""},
		{"a grid of shaded grounds", "0.9 g\n100 670 150 30 re f\n250 670 150 30 re f\n100 640 150 30 re f\n250 640 150 30 re f\n0 g\n",
			cellTexts(sixWords[:4]...), ""},
		{"a grid holding no text", ruledLines(), drawText(100, 580, "Under an empty grid."), ""},
		{"a grid whose text is turned", ruledLines(), "BT /F1 12 Tf 0 1 -1 0 120 646 Tm (Turned) Tj ET\n" + drawText(256, 680, "Upright"), turnedTableNote},
	} {
		got, pr := proposed(t, ruledFixture(c.drawn+c.text))
		bare, barePr := proposed(t, ruledFixture(c.text))
		if !reflect.DeepEqual(got, bare) {
			t.Errorf("%s: the drawing changed the proposal:\n  with    %v\n  without %v", c.name, got, bare)
		}
		for _, row := range got {
			if strings.HasPrefix(row, "Table") || strings.HasPrefix(row, "TR") || strings.HasPrefix(row, "TH") || strings.HasPrefix(row, "TD") {
				t.Errorf("%s: proposed %s", c.name, row)
			}
		}
		want := tableNotes(barePr)
		if c.note != "" {
			want = []string{c.note}
			if len(barePr.Unsupported) > 0 {
				want = []string{barePr.Unsupported[0].Reason + "; " + c.note}
			}
		}
		if notes := tableNotes(pr); !reflect.DeepEqual(notes, want) {
			t.Errorf("%s: the page is reported %q, want %q", c.name, notes, want)
		}
	}
}

// gridRules draws every rule of the grid with these column and row boundaries, rows from the top down.
func gridRules(xs, ys []float64) string {
	var b strings.Builder
	for _, y := range ys {
		b.WriteString(hRule(xs[0], xs[len(xs)-1], y))
	}
	for _, x := range xs {
		b.WriteString(vRule(x, ys[len(ys)-1], ys[0]))
	}
	return b.String()
}

// TestAGridWithFewerThanAQuarterOfItsCellsFilledIsNotATable — ADR-123: a worksheet's ruling with a word here
// and there proposes exactly what its text proposes with nothing drawn, and says nothing on its page; a grid
// with a quarter of its cells filled, or more, is a table still.
func TestAGridWithFewerThanAQuarterOfItsCellsFilledIsNotATable(t *testing.T) {
	two, three := []float64{100, 250, 400}, []float64{100, 200, 300, 400}
	for _, c := range []struct {
		name   string
		xs, ys []float64
		filled [][2]int // row, column
		table  bool
	}{
		{"two cells of nine", three, gridYs, [][2]int{{0, 0}, {1, 1}}, false},
		{"one cell of six", two, gridYs, [][2]int{{0, 0}}, false},
		{"one cell of four", two, gridYs[:3], [][2]int{{0, 0}}, true},
		{"three cells of nine", three, gridYs, [][2]int{{0, 0}, {1, 1}, {2, 2}}, true},
		// Two words in one cell fill one cell.
		{"two words in one cell of six", two, gridYs, [][2]int{{0, 0}, {0, 0}}, false},
	} {
		text := ""
		for i, f := range c.filled {
			text += drawText(c.xs[f[1]]+6+30*float64(i), c.ys[f[0]+1]+10, fmt.Sprintf("W%d", i))
		}
		got, pr := proposed(t, ruledFixture(gridRules(c.xs, c.ys)+text))
		if c.table {
			if len(got) == 0 || !strings.HasPrefix(got[0], "Table") {
				t.Errorf("%s: proposed %v, want a table", c.name, got)
			}
			continue
		}
		bare, barePr := proposed(t, ruledFixture(text))
		if !reflect.DeepEqual(got, bare) {
			t.Errorf("%s: the grid changed the proposal:\n  with    %v\n  without %v", c.name, got, bare)
		}
		if notes, want := tableNotes(pr), tableNotes(barePr); !reflect.DeepEqual(notes, want) {
			t.Errorf("%s: the page is reported %q, want %q", c.name, notes, want)
		}
	}
}

// TestRulesAHalfPointApartAreOneRule — tiles set a half point apart: each pair of neighbouring sides is one
// rule, at the middle of the two.
func TestRulesAHalfPointApartAreOneRule(t *testing.T) {
	tiles := ""
	for r := 0; r < 3; r++ {
		for c := 0; c < 2; c++ {
			tiles += fmt.Sprintf("%g %g %g %g re S\n", gridXs[c]+0.25, gridYs[r+1]+0.25, gridXs[c+1]-gridXs[c]-0.5, gridYs[r]-gridYs[r+1]-0.5)
		}
	}
	got, pr := proposed(t, ruledFixture(tiles+cellTexts(sixWords...)))
	if len(got) != 10 || got[0] != `Table^-1 "Name Qty Apple 3 Pear 5"` || got[9] != `TD^7 "5"` {
		t.Fatalf("proposed %v", got)
	}
	if r := pr.Elements[6].Rect; r != [4]float64{250, 640, 399.75, 670} {
		t.Errorf("the second row's second cell's box is %v — inner sides at the middle of each pair, outer sides where drawn", r)
	}
}

// TestOverlappingGridsAreReportedNotRead — a grid drawn inside a cell of another: a run would belong to both.
func TestOverlappingGridsAreReportedNotRead(t *testing.T) {
	inner := ""
	for _, y := range []float64{664, 655, 646} {
		inner += hRule(110, 230, y)
	}
	for _, x := range []float64{110, 170, 230} {
		inner += vRule(x, 646, 664)
	}
	_, pr := proposed(t, ruledFixture(ruledLines()+inner+cellTexts("Name", "Qty", "", "3", "Pear", "5")+
		"BT /F1 6 Tf 112 657 Td (a) Tj ET\n"))
	for _, e := range pr.Elements {
		if tableRoles[e.Role] {
			t.Errorf("proposed %s %q from grids that overlap", e.Role, e.Text)
		}
	}
	if notes := tableNotes(pr); len(notes) != 1 || !strings.Contains(notes[0], nestedTablesNote) {
		t.Errorf("the page is reported %q, want %q", notes, nestedTablesNote)
	}
}

// TestAPageOfRulesBeyondTheBoundIsProposedWithoutTables — the comparisons made for one page are bounded.
func TestAPageOfRulesBeyondTheBoundIsProposedWithoutTables(t *testing.T) {
	grid := func(n int) []pageShape {
		var out []pageShape
		for i := 0; i <= n; i++ {
			at := float64(i) * 4
			out = append(out, pageShape{box: [4]float64{0, at, float64(n) * 4, at}, stroked: true},
				pageShape{box: [4]float64{at, 0, at, float64(n) * 4}, stroked: true})
		}
		return out
	}
	within, beyond := 499, 500 // 500×500 crossings is the bound; 501×501 is past it
	if (within+1)*(within+1) > gridMaxCrossings || (beyond+1)*(beyond+1) <= gridMaxCrossings {
		t.Fatalf("setup: %d and %d rules a side do not straddle the bound %d", within+1, beyond+1, gridMaxCrossings)
	}
	if g, _ := ruledGrids(grid(within)); len(g) != 1 || g[0].rows() != within || g[0].cols() != within {
		t.Errorf("a grid at the bound read as %d grid(s)", len(g))
	}
	if g, m := ruledGrids(grid(beyond)); len(g) != 0 || len(m) != 0 {
		t.Errorf("a grid past the bound read as %d grid(s) and %d merged", len(g), len(m))
	}
}

// TestATableSitsWhereItsTopLeftCellWould — text above a table comes before it and text below after; text
// standing wholly to its left is read first and text wholly to its right after, as columns are.
func TestATableSitsWhereItsTopLeftCellWould(t *testing.T) {
	order := func(content string) string {
		_, pr := proposed(t, ruledFixture(content))
		var out []string
		for _, e := range pr.Elements {
			if e.Parent == -1 {
				out = append(out, e.Role+":"+e.Text)
			}
		}
		return strings.Join(out, " | ")
	}
	table := ruledLines() + cellTexts(sixWords...)
	const tbl = "Table:Name Qty Apple 3 Pear 5"
	for _, c := range []struct{ name, content, want string }{
		{"above and below", drawText(100, 740, "Above the table.") + table + drawText(100, 580, "Below the table."),
			"P:Above the table. | " + tbl + " | P:Below the table."},
		{"drawn in another order", drawText(100, 580, "Below the table.") + table + drawText(100, 740, "Above the table."),
			"P:Above the table. | " + tbl + " | P:Below the table."},
		{"text beside it, to the right", table + drawText(430, 680, "Beside it."), tbl + " | P:Beside it."},
		{"text beside it, to the left", table + drawText(20, 650, "Beside it."), "P:Beside it. | " + tbl},
		{"alone", table, tbl},
		// A page number at the top right is a running header, not a column to the table's right.
		{"under a running header", drawText(450, 770, "Page 1") + drawText(100, 750, "A first line of the body.") +
			drawText(100, 736, "A second line of the body.") + drawText(100, 722, "A third line of it.") + table,
			"P:Page 1 | P:A first line of the body. | P:A second line of the body. A third line of it. | " + tbl},
	} {
		if got := order(c.content); got != c.want {
			t.Errorf("%s:\n  got  %s\n  want %s", c.name, got, c.want)
		}
	}

	// Two tables on a page are proposed top first, whichever was drawn first.
	lower := ""
	for _, y := range []float64{560, 530, 500} {
		lower += hRule(100, 400, y)
	}
	for _, x := range gridXs {
		lower += vRule(x, 500, 560)
	}
	lower += drawText(106, 540, "Lower") + drawText(256, 540, "table") + drawText(106, 510, "x") + drawText(256, 510, "y")
	if got, want := order(lower+table), tbl+" | Table:Lower table x y"; got != want {
		t.Errorf("two tables:\n  got  %s\n  want %s", got, want)
	}

	// A table ends a list: the items above it and the items below it are two lists.
	_, pr := proposed(t, ruledFixture(drawText(100, 740, "- one")+table+drawText(100, 580, "- two")))
	if n := len(pr.Elements); n != 12 || pr.Elements[0].Role != "LI" || pr.Elements[11].Role != "LI" {
		t.Fatalf("setup: the list fixture proposes %d element(s)", n)
	}
	if pr.Elements[0].List != 0 || pr.Elements[11].List != 1 {
		t.Errorf("the items either side of a table are in lists %d and %d, want 0 and 1", pr.Elements[0].List, pr.Elements[11].List)
	}
}

// TestATablesTextStillCountsTowardTheBodySize — a document that is mostly table has the table's size as its
// body, as it did when the table was proposed as paragraphs, so the larger line above it is a heading.
func TestATablesTextStillCountsTowardTheBodySize(t *testing.T) {
	_, pr := proposed(t, ruledFixture("BT /F1 16 Tf 100 740 Td (Stock) Tj ET\n"+ruledLines()+cellTexts(sixWords...)))
	if pr.Elements[0].Role != "H1" || pr.Elements[0].Text != "Stock" {
		t.Errorf("the 16pt line over a 12pt table is proposed %s %q, want H1", pr.Elements[0].Role, pr.Elements[0].Text)
	}
}

// tableRead is a committed table as a reader finds it: each row's cells as kind, scope and text.
func tablesRead(t *testing.T, pdf []byte) [][][]string {
	t.Helper()
	tree, err := ReadStructure(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var rowsOf func(i int) [][]string
	rowsOf = func(i int) [][]string {
		var rows [][]string
		for _, k := range tree.Elements[i].Kids {
			e := tree.Elements[k]
			if e.Standard != "TR" {
				rows = append(rows, rowsOf(k)...)
				continue
			}
			var row []string
			for _, ck := range e.Kids {
				c := tree.Elements[ck]
				cell := c.Standard
				if c.Scope != "" {
					cell += "/" + c.Scope
				}
				row = append(row, cell+" "+squeeze(c.Text))
			}
			rows = append(rows, row)
		}
		return rows
	}
	var out [][][]string
	for i, e := range tree.Elements {
		if e.Standard == "Table" {
			out = append(out, rowsOf(i))
		}
	}
	return out
}

// TestACommittedTableReadsBackNested — the table as proposed, and as a reviewer retypes its cells: a header
// in the first row heads its column, one in the first column of a later row heads its row, and a data cell
// has no scope.
func TestACommittedTableReadsBackNested(t *testing.T) {
	src := ruledFixture(drawText(100, 740, "Above the table.") + ruledLines() + cellTexts("Name", "Qty", "Apple", "", "Pear", "5"))
	_, pr := proposed(t, src)
	out, err := CommitTags(src, reviewAll(pr))
	if err != nil {
		t.Fatal(err)
	}
	want := [][][]string{{
		{"TH/Column Name", "TH/Column Qty"},
		{"TD Apple", "TD "},
		{"TD Pear", "TD 5"},
	}}
	if got := tablesRead(t, out); !reflect.DeepEqual(got, want) {
		t.Errorf("the committed table reads\n  %v\nwant\n  %v", got, want)
	}
	// The nesting itself: each TR directly under the Table, the Table under the root, after the paragraph.
	tree, err := ReadStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	var top []string
	for i, e := range tree.Elements {
		if e.Parent == -1 {
			top = append(top, e.Standard)
		}
		if e.Standard == "TR" && tree.Elements[e.Parent].Standard != "Table" {
			t.Errorf("element %d: a TR sits under %s", i, tree.Elements[e.Parent].Standard)
		}
	}
	if strings.Join(top, " ") != "P Table" {
		t.Errorf("the root holds %v, want a paragraph and then the table", top)
	}

	rv := reviewAll(pr) // P Table TR TH TH TR TD TD TR TD TD
	rv[3].Role = "TD"   // Name: a data cell after all
	rv[6].Role = "TH"   // Apple: heads its row
	rv[10].Role = "TH"  // 5: a header that is in neither the first row nor the first column
	out, err = CommitTags(src, rv)
	if err != nil {
		t.Fatal(err)
	}
	want = [][][]string{{
		{"TD Name", "TH/Column Qty"},
		{"TH/Row Apple", "TD "},
		{"TD Pear", "TH/Column 5"},
	}}
	if got := tablesRead(t, out); !reflect.DeepEqual(got, want) {
		t.Errorf("the retyped table reads\n  %v\nwant\n  %v", got, want)
	}
}

// TestATableReviewKeepsTheTableWhole — every refusal a table adds to the review, each by its own sentence;
// then what a reviewer may do: move the table whole, and ignore it whole.
func TestATableReviewKeepsTheTableWhole(t *testing.T) {
	src := ruledFixture(drawText(100, 740, "Above the table.") + ruledLines() + cellTexts(sixWords...) + drawText(100, 580, "Below the table."))
	_, pr := proposed(t, src)
	// 0 P · 1 Table · 2 TR · 3 TH · 4 TH · 5 TR · 6 TD · 7 TD · 8 TR · 9 TD · 10 TD · 11 P
	if len(pr.Elements) != 12 || pr.Elements[1].Role != "Table" || pr.Elements[11].Role != "P" {
		t.Fatalf("setup: the fixture proposes %d element(s)", len(pr.Elements))
	}
	swap := func(i, j int) func([]TagReview) []TagReview {
		return func(r []TagReview) []TagReview { r[i], r[j] = r[j], r[i]; return r }
	}
	set := func(i int, role string) func([]TagReview) []TagReview {
		return func(r []TagReview) []TagReview { r[i].Role = role; return r }
	}
	ignore := func(i int) func([]TagReview) []TagReview {
		return func(r []TagReview) []TagReview { r[i].Ignore = true; return r }
	}
	const (
		parted  = "a table's rows and cells stay under it in the order they were proposed"
		whole   = "a row or a cell cannot be ignored by itself"
		cell    = "a table cell is a header cell (TH) or a data cell (TD)"
		keeps   = "a table and its rows keep their type"
		nobody  = "is not a role a reviewer can choose"
		nothing = "every element was ignored"
	)
	for _, c := range []struct {
		name string
		edit func([]TagReview) []TagReview
		want string
	}{
		{"two cells of a row exchanged", swap(3, 4), parted},
		{"two rows exchanged", func(r []TagReview) []TagReview {
			return append(append(append(append([]TagReview{}, r[:2]...), r[5:8]...), r[2:5]...), r[8:]...)
		}, parted},
		{"a row before its table", func(r []TagReview) []TagReview {
			return append([]TagReview{r[0], r[2], r[1]}, r[3:]...)
		}, parted},
		{"a paragraph moved into the table", func(r []TagReview) []TagReview {
			return append(append(append([]TagReview{}, r[:5]...), r[11]), r[5:11]...)
		}, parted},
		{"a cell moved out of the table", func(r []TagReview) []TagReview {
			return append(append(append([]TagReview{}, r[:10]...), r[11]), r[10])
		}, parted},
		{"a row ignored", ignore(5), whole},
		{"a cell ignored", ignore(6), whole},
		{"a cell ignored with its table", func(r []TagReview) []TagReview { r[1].Ignore, r[6].Ignore = true, true; return r }, whole},
		{"a cell made a paragraph", set(6, "P"), cell},
		{"a cell made a row", set(6, "TR"), cell},
		{"a table made a heading", set(1, "H1"), keeps},
		{"a row made a cell", set(2, "TD"), keeps},
		{"a row made a table", set(5, "Table"), keeps},
		{"a paragraph made a cell", set(0, "TD"), nobody},
		{"a paragraph made a table", set(11, "Table"), nobody},
		{"everything ignored", func(r []TagReview) []TagReview { r[0].Ignore, r[1].Ignore, r[11].Ignore = true, true, true; return r }, nothing},
	} {
		_, err := CommitTags(src, c.edit(reviewAll(pr)))
		if !errors.Is(err, ErrTagsReview) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want a refused review saying %q", c.name, err, c.want)
		}
	}
	// A cell's text is echoed like any element's: a table that changed is stale, not malformed.
	stale := reviewAll(pr)
	stale[6].Text = "Apples"
	if _, err := CommitTags(src, stale); !errors.Is(err, ErrTagsStale) {
		t.Errorf("a cell whose text changed: err = %v, want stale", err)
	}

	// The table moved whole, to the end.
	rv := reviewAll(pr)
	moved := append([]TagReview{rv[0], rv[11]}, rv[1:11]...)
	out, err := CommitTags(src, moved)
	if err != nil {
		t.Fatalf("moving the whole table: %v", err)
	}
	tree, err := ReadStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	var top []string
	for _, e := range tree.Elements {
		if e.Parent == -1 {
			top = append(top, e.Standard)
		}
	}
	if strings.Join(top, " ") != "P P Table" {
		t.Errorf("the root holds %v after the table was moved last", top)
	}
	if got := tablesRead(t, out); len(got) != 1 || len(got[0]) != 3 || got[0][2][1] != "TD 5" {
		t.Errorf("the moved table reads %v", got)
	}

	// The table ignored: its text is an artifact, and the rest is committed.
	rv = reviewAll(pr)
	rv[1].Ignore = true
	out, err = CommitTags(src, rv)
	if err != nil {
		t.Fatalf("ignoring the whole table: %v", err)
	}
	if got := tablesRead(t, out); len(got) != 0 {
		t.Errorf("an ignored table was written: %v", got)
	}
	ctx, err := inspectionRead(out)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := readPageRuns(ctx, pageAt(ctx, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, tagged := 0, 0
	for _, r := range runs.runs {
		switch {
		case r.artifact && r.mcid < 0:
			artifacts++
		case r.mcid >= 0:
			tagged++
		default:
			t.Errorf("%q is neither tagged nor an artifact", r.text)
		}
	}
	if artifacts != 6 || tagged != 2 {
		t.Errorf("%d run(s) are artifacts and %d tagged, want the table's 6 and the 2 paragraphs", artifacts, tagged)
	}
}

// TestIgnoringOneTableLeavesTheNextToBeWritten — an ignore is the table's it is set on, not the next one's.
func TestIgnoringOneTableLeavesTheNextToBeWritten(t *testing.T) {
	lower := ""
	for _, y := range []float64{560, 530, 500} {
		lower += hRule(100, 400, y)
	}
	for _, x := range gridXs {
		lower += vRule(x, 500, 560)
	}
	lower += drawText(106, 540, "Lower") + drawText(256, 540, "table") + drawText(106, 510, "x") + drawText(256, 510, "y")
	src := ruledFixture(ruledLines() + cellTexts(sixWords...) + lower)
	_, pr := proposed(t, src)
	if len(pr.Elements) != 17 || pr.Elements[10].Role != "Table" {
		t.Fatalf("setup: the fixture proposes %d element(s)", len(pr.Elements))
	}
	rv := reviewAll(pr)
	rv[0].Ignore = true
	out, err := CommitTags(src, rv)
	if err != nil {
		t.Fatal(err)
	}
	want := [][][]string{{{"TH/Column Lower", "TH/Column table"}, {"TD x", "TD y"}}}
	if got := tablesRead(t, out); !reflect.DeepEqual(got, want) {
		t.Errorf("the committed tables read %v, want only the second: %v", got, want)
	}
}

// TestTheCommitRefusesATablesElementOutsideItsTable — the writer takes a table's nesting from the order of its
// elements, so an order that is not a table's is refused here too, whatever let it through.
func TestTheCommitRefusesATablesElementOutsideItsTable(t *testing.T) {
	src := ruledFixture(drawText(100, 740, "Above the table.") + ruledLines() + cellTexts(sixWords...))
	p := proposeFor(t, src)
	el := p.elements // P Table TR TH TH TR TD TD TR TD TD
	for _, c := range []struct {
		name string
		seq  []proposedElement
	}{
		{"a row with no table", []proposedElement{el[0], el[2], el[3], el[4]}},
		{"a cell with no row", []proposedElement{el[1], el[3]}},
		{"a row after a paragraph that ended its table", []proposedElement{el[1], el[2], el[3], el[4], el[0], el[5], el[6], el[7]}},
		{"a cell after a paragraph that ended its row", []proposedElement{el[1], el[2], el[3], el[0], el[4]}},
		{"a cell under the row of the table before", []proposedElement{el[1], el[2], el[3], el[4], el[1], el[6]}},
	} {
		if _, err := commitProposal(src, c.seq); !errors.Is(err, errCommitStale) || !strings.Contains(err.Error(), "outside a table's rows") {
			t.Errorf("%s: err = %v, want a refusal naming the element outside a table's rows", c.name, err)
		}
	}
}

// ruledTableODT is `tableAndFigureODT`'s table with its cells' borders drawn — LibreOffice's own tagged table,
// ruled. (That document's table has no borders, and LibreOffice then paints nothing at all for it.)
func ruledTableODT(t *testing.T) []byte {
	t.Helper()
	cell := func(s string) string {
		return `<table:table-cell table:style-name="C" office:value-type="string"><text:p>` + s + `</text:p></table:table-cell>`
	}
	body := `<text:h text:outline-level="1">Report</text:h><text:p>Intro paragraph.</text:p>` +
		`<table:table table:name="T1"><table:table-column table:number-columns-repeated="2"/>` +
		`<table:table-header-rows><table:table-row>` + cell("Name") + cell("Qty") + `</table:table-row></table:table-header-rows>` +
		`<table:table-row>` + cell("Apple") + cell("3") + `</table:table-row>` +
		`<table:table-row>` + cell("Pear") + cell("5") + `</table:table-row></table:table>` +
		`<text:p>After the table.</text:p>`
	style := `<style:style style:name="C" style:family="table-cell">` +
		`<style:table-cell-properties fo:border="0.5pt solid #000000" fo:padding="0.1cm"/></style:style>`
	pdf, err := ConvertOfficeToPDF(odtDocument(t, style, body), "odt")
	if err != nil {
		t.Fatalf("LibreOffice is present and could not convert the ruled-table document: %v", err)
	}
	return pdf
}

// tableClauses are veraPDF's PDF/UA-1 table rules.
var tableClauses = []string{"7.2 t15", "7.2 t41", "7.2 t42", "7.2 t43", "7.5 t1", "7.5 t2"}

// TestAProducersRuledTableIsProposedAsTheProducerTaggedIt — LibreOffice's own table, its tags removed: the
// proposal finds the rows, columns, header row and cell texts LibreOffice wrote, the commit writes them back,
// and veraPDF fails no table clause and nothing the same document committed WITHOUT tables does not fail.
func TestAProducersRuledTableIsProposedAsTheProducerTaggedIt(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is not installed, so no producer's ruled table is proposed in this run; the hand-built grids still run.")
	}
	original := ruledTableODT(t)
	truth := tablesRead(t, original)
	if len(truth) != 1 || len(truth[0]) != 3 || len(truth[0][0]) != 2 || truth[0][0][0] != "TH/Column Name" || truth[0][2][1] != "TD 5" {
		t.Fatalf("setup: LibreOffice tagged its table as %v", truth)
	}
	src, err := RemoveStructure(original)
	if err != nil {
		t.Fatal(err)
	}
	rows, pr := proposed(t, src)
	// The proposal against the producer's own structure: the same cells, of the same kind, row by row.
	var got [][]string
	for _, e := range pr.Elements {
		switch {
		case e.Role == "TR":
			got = append(got, nil)
		case cellRoles[e.Role]:
			got[len(got)-1] = append(got[len(got)-1], e.Role+" "+squeeze(e.Text))
		}
	}
	var want [][]string
	for _, row := range truth[0] {
		var w []string
		for _, c := range row {
			w = append(w, strings.Replace(c, "/Column", "", 1))
		}
		want = append(want, w)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the proposal's table is\n  %v\nLibreOffice's own is\n  %v\n(proposal: %v)", got, want, rows)
	}
	tables := 0
	for _, e := range pr.Elements {
		if e.Role == "Table" {
			tables++
		}
	}
	if tables != 1 || len(pr.Unsupported) != 0 {
		t.Errorf("%d table(s) proposed, page notes %v", tables, pr.Unsupported)
	}
	out, err := CommitTags(src, reviewAll(pr))
	if err != nil {
		t.Fatal(err)
	}
	if back := tablesRead(t, out); !reflect.DeepEqual(back, truth) {
		t.Errorf("the committed table reads\n  %v\nLibreOffice's own read\n  %v", back, truth)
	}

	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the committed table's ua1 clauses are UNCHECKED in this run.")
	}
	// The same document committed without tables: its pages read through the door that knows none.
	ctx, err := inspectionRead(src)
	if err != nil {
		t.Fatal(err)
	}
	l, err := readPageLayout(ctx, pageAt(ctx, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	flat := proposeFromLayouts([]pageLayout{l})
	for _, el := range flat.elements {
		if tableRoles[el.role] {
			t.Fatalf("setup: the layout door proposed a %s", el.role)
		}
	}
	baseline, err := commitProposal(src, flat.elements)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"flat.pdf": baseline, "table.pdf": out} {
		f := filepath.Join(dir, n)
		if werr := os.WriteFile(f, b, 0o600); werr != nil {
			t.Fatal(werr)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	before, after := cl["flat.pdf"], cl["table.pdf"]
	if before == nil || after == nil {
		t.Fatal("veraPDF could not validate one of the pair, so there is no differential")
	}
	t.Logf("committed without tables fails %v; with the table %v", sortedClauses(before), sortedClauses(after))
	for _, c := range tableClauses {
		if after[c] {
			t.Errorf("the committed table fails %s", c)
		}
	}
	for c := range after {
		if !before[c] {
			t.Errorf("proposing the table ADDED ua1 clause %s", c)
		}
	}
}

// TestAHandBuiltTableFailsNoTableClause — the shapes the producer's table does not have: an empty cell, and
// headers a reviewer placed in a column and in the body.
func TestAHandBuiltTableFailsNoTableClause(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so a committed table with an empty cell is UNCHECKED against ua1 in this run.")
	}
	src := ruledFixture(ruledLines() + cellTexts("Name", "Qty", "Apple", "", "Pear", "5"))
	_, pr := proposed(t, src)
	asProposed, err := CommitTags(src, reviewAll(pr))
	if err != nil {
		t.Fatal(err)
	}
	rv := reviewAll(pr) // Table TR TH TH TR TD TD TR TD TD
	rv[5].Role, rv[9].Role = "TH", "TH"
	retyped, err := CommitTags(src, rv)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"proposed.pdf": asProposed, "retyped.pdf": retyped} {
		f := filepath.Join(dir, n)
		if werr := os.WriteFile(f, b, 0o600); werr != nil {
			t.Fatal(werr)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	for _, n := range []string{"proposed.pdf", "retyped.pdf"} {
		failed := cl[n]
		if failed == nil {
			t.Fatalf("veraPDF could not validate %s", n)
		}
		t.Logf("%s fails %v", n, sortedClauses(failed))
		for _, c := range append([]string{"7.1 t3"}, tableClauses...) {
			if failed[c] {
				t.Errorf("%s fails %s", n, c)
			}
		}
	}
}

// TestATableWithNoRulesIsNotProposed — LibreOffice's table with no borders paints nothing for the table, and
// text that only lines up is never a table here: it is proposed as paragraphs, and nothing is reported.
func TestATableWithNoRulesIsNotProposed(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is not installed, so its unruled table is not proposed in this run.")
	}
	src, err := RemoveStructure(tableAndFigureODT(t))
	if err != nil {
		t.Fatal(err)
	}
	rows, pr := proposed(t, src)
	for _, e := range pr.Elements {
		if e.Parent != -1 || tableRoles[e.Role] {
			t.Errorf("an unruled table proposed %s under %d: %v", e.Role, e.Parent, rows)
		}
	}
	if len(pr.Unsupported) != 0 {
		t.Errorf("the page is reported %v", pr.Unsupported)
	}
}

// TestATableReviewedAsNotOneIsWrittenAsParagraphs — a ruled form tiles as regularly as a table. A reviewer who
// says the grid is not a table (the Table reviewed as P) gets each cell that has text as a paragraph, row by
// row, in place; no Table, row or cell is written, an empty cell writes nothing, and none of the text becomes
// decoration — which is all ignoring the table could have offered.
func TestATableReviewedAsNotOneIsWrittenAsParagraphs(t *testing.T) {
	words := append([]string{}, sixWords...)
	words[3] = "" // an empty cell
	src := ruledFixture(drawText(100, 740, "Above the table.") + ruledLines() + cellTexts(words...) + drawText(100, 580, "Below the table."))
	_, pr := proposed(t, src)
	if len(pr.Elements) != 12 || pr.Elements[1].Role != "Table" {
		t.Fatalf("setup: the fixture proposes %d element(s)", len(pr.Elements))
	}
	reviews := make([]TagReview, len(pr.Elements))
	for i, e := range pr.Elements {
		reviews[i] = TagReview{ID: e.ID, Role: e.Role, Text: e.Text}
	}
	reviews[1].Role = "P"
	out, err := CommitTags(src, reviews)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range viewOf(t, out).elements {
		got = append(got, e.kind+":"+e.text)
	}
	want := []string{"P:Above the table."}
	for _, w := range words {
		if w != "" {
			want = append(want, "P:"+w)
		}
	}
	want = append(want, "P:Below the table.")
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the tree reads\n %s\nwant\n %s", strings.Join(got, "|"), strings.Join(want, "|"))
	}
	if n, err := UnmarkedTextRuns(out); err != nil || n != 0 {
		t.Errorf("%d run(s) are neither tagged nor an artifact (%v)", n, err)
	}
	before, after := marksOf(t, src), marksOf(t, out)
	if after.artifact != before.artifact {
		t.Errorf("%d run(s) of text became decoration when the table was declined, want none", after.artifact-before.artifact)
	}
	// Ignored and declined at once is ignored: the box a reviewer ticked wins.
	reviews[1].Ignore = true
	if ignored, err := CommitTags(src, reviews); err != nil {
		t.Fatal(err)
	} else if n := len(viewOf(t, ignored).elements); n != 2 {
		t.Errorf("an ignored table reviewed as P left %d element(s), want the two paragraphs outside it", n)
	}
}
