package uacheck

import (
	"math/rand"
	"strings"
	"testing"
)

// P03.S04 — every table below was run through veraPDF 1.30.2 before it was pinned, and each verdict is
// veraPDF's. The corpus has only fail files for 7.2 t41 (whose one file fails 7.2-42 in fact) and t43.

func TestTableGeometryAgreesWithVeraPDFBothWays(t *testing.T) {
	const regular = "Document(Table(TR(TH!r2!scope=Row,TH!c2!scope=Column),TR(TD,TD)))"
	for _, tc := range []struct{ clause, fail string }{
		{"7.2 t15", "Document(Table(TR(TD,TD!r2),TR(TD!c2)))"},
		{"7.2 t41", "Document(Table(TR(TD,TD!r3),TR(TD,TD)))"},
		{"7.2 t42", "Document(Table(TR(TD,TD),TR(TD,TD,TD)))"},
		{"7.2 t43", "Document(Table(TR(TD,TD),TR(TD)))"},
		{"7.5 t1", "Document(Table(TR(TH!id=a,TH!id=b),TR(TD,TD)))"},
		{"7.5 t2", "Document(Table(TR(TH!id=a,TH!id=b),TR(TD!headers=zz,TD)))"},
	} {
		if got := verdictOf(t, treeDoc("", regular), tc.clause); got.Verdict != Pass {
			t.Errorf("%s over a regular spanned table = %v (%s), want Pass", tc.clause, got.Verdict, got.Why)
		}
		if got := verdictOf(t, treeDoc("", tc.fail), tc.clause); got.Verdict != Fail {
			t.Errorf("%s over %s = %v (%s), want Fail", tc.clause, tc.fail, got.Verdict, got.Why)
		}
	}
}

// TestTheLayoutIsVeraPDFsStepForStep — the edges `GFSETable.checkTable` decides, each measured.
func TestTheLayoutIsVeraPDFsStepForStep(t *testing.T) {
	for _, tc := range []struct {
		name, spec, clause string
		want               Verdict
	}{
		{"a row with no cells falls short at width 0", "Document(Table(TR(TD,TD),TR))", "7.2 t43", Fail},
		{"an overflow is t42, never t43", "Document(Table(TR(TD,TD),TR(TD,TD,TD)))", "7.2 t43", Pass},
		{"a short row is t43, never t42", "Document(Table(TR(TD,TD),TR(TD)))", "7.2 t42", Pass},
		{"the first error ends the layout: an intersection is not also a short row", "Document(Table(TR(TD,TD!r2),TR(TD!c2)))", "7.2 t43", Pass},
		{"a ColSpan of 0 places nothing and the row still fills", "Document(Table(TR(TD,TD),TR(TD!c0,TD,TD)))", "7.2 t42", Pass},
		{"a header matched by ID connects the cell", "Document(Table(TR(TH!id=a,TH!id=b),TR(TD!headers=a,TD!headers=b)))", "7.5 t1", Pass},
		{"a TD directly under a Column-scoped TH is connected", "Document(Table(TR(TD,TH!scope=Column),TR(TH,TD)))", "7.5 t1", Pass},
		// A Scope connects only in its own direction, measured both ways.
		{"a Column-scoped TH to the LEFT does not connect", "Document(Table(TR(TH,TH),TR(TH!scope=Column,TD)))", "7.5 t1", Fail},
		{"a Row-scoped TH ABOVE does not connect", "Document(Table(TR(TH,TH!scope=Row),TR(TH,TD)))", "7.5 t1", Fail},
		{"the data cell at [0][0] is never asked", "Document(Table(TR(TD,TH!scope=Column),TR(TH!scope=Row,TD)))", "7.5 t1", Pass},
		{"only the FIRST headerless cell is flagged, so unknown Headers after it pass", "Document(Table(TR(TH,TH),TR(TD,TD!headers=zz)))", "7.5 t2", Pass},
		// veraPDF 1.30.2 throws ArrayIndexOutOfBoundsException here: row 2 lies past the one row it counted.
		{"a table veraPDF's own layout cannot place is CannotCheck", "Document(Table(TR(TD!r0,TD),TR(TD,TD)))", "7.2 t42", CannotCheck},
		{"no Table is not applicable", "Document(P)", "7.2 t41", NotApplicable},
		{"a TD outside any Table passes", "Document(TD)", "7.5 t1", Pass},
	} {
		got := verdictOf(t, treeDoc("", tc.spec), tc.clause)
		if got.Verdict != tc.want {
			t.Errorf("%s: %s over %s = %v (%s), want %v", tc.name, tc.clause, tc.spec, got.Verdict, got.Why, tc.want)
		}
		// A rule that panics is reported as CannotCheck too, so the unlayable verdict must carry the layout's
		// own reason — found when removing the guard left this row green through a recovered nil dereference.
		if tc.want == CannotCheck && !strings.Contains(got.Why, "rows veraPDF counts") {
			t.Errorf("%s: the reason %q is not the layout's; a recovered panic reads as CannotCheck too", tc.name, got.Why)
		}
	}
}

// TestTheLayoutSurvivesWhatVeraPDFSurvives — found by P03.S04's review and measured on veraPDF 1.30.2. veraPDF
// truncates its running counts to 32 bits (Java `int += long`), so a crafted span wraps rather than sizing
// the grid; an untruncated port asked for 103 GB and the process died unrecoverably. Each verdict is veraPDF's,
// except where veraPDF itself runs out of memory, where nib's answer is CannotCheck and it returns at once.
func TestTheLayoutSurvivesWhatVeraPDFSurvives(t *testing.T) {
	for _, tc := range []struct {
		name, spec, clause string
		want               Verdict
	}{
		{"a RowSpan of 2^32+1 wraps the height to 1 and fails t41", "Document(Table(TR(TD!r4294967297,TD)))", "7.2 t41", Fail},
		{"a ColSpan of -(2^32-1) wraps the width to 1 and the next row falls short", "Document(Table(TR(TD!c-4294967295),TR(TD)))", "7.2 t43", Fail},
		// veraPDF 1.30.2: OutOfMemory, "Java heap space".
		{"a grid of two billion slots is not laid out", "Document(Table(TR(TD!c1000000000),TR(TD)))", "7.2 t42", CannotCheck},
		{"one empty-string Headers entry joins to '' — t1, not t2", "Document(Table(TR(TH,TH),TR(TD!headers=,TD)))", "7.5 t1", Fail},
		{"one empty-string Headers entry is not an unknown header", "Document(Table(TR(TH,TH),TR(TD!headers=,TD)))", "7.5 t2", Pass},
		{"an empty-name Scope still scopes the header", "Document(Table(TR(TH!scope=,TH!scope=),TR(TD,TD)))", "7.5 t1", Pass},
	} {
		got := verdictOf(t, treeDoc("", tc.spec), tc.clause)
		if got.Verdict != tc.want {
			t.Errorf("%s: %s = %v (%s), want %v", tc.name, tc.clause, got.Verdict, got.Why, tc.want)
		}
		// A recovered panic reads as CannotCheck too, so the refusal must be the layout's own.
		if tc.want == CannotCheck && !strings.Contains(got.Why, "more than nib lays out") {
			t.Errorf("%s: the reason %q is not the layout's slot cap; a recovered panic reads as CannotCheck too", tc.name, got.Why)
		}
	}
}

// bruteHeaderInLine is the geometric header search as it was written before the P07 phase close — a scan per
// cell — kept here as the reference `headerReach` must agree with on every cell.
func bruteHeaderInLine(d *Document, grid [][]*tableCell, cell *tableCell) bool {
	scope := func(c *tableCell) string {
		if c.std != "TH" {
			return ""
		}
		return d.name(d.tableAttributeOfType(c.dict, "Scope", attrName))
	}
	if cell.row > 0 {
		for col := cell.col; col < cell.col+int(cell.colSpan); col++ {
			seen := false
			for r := cell.row - 1; r >= 0; r-- {
				if sc := scope(grid[r][col]); sc == "Both" || sc == "Column" {
					return true
				}
				if grid[r][col].std == "TH" {
					seen = true
				} else if seen {
					break
				}
			}
		}
	}
	if cell.col > 0 {
		for r := cell.row; r < cell.row+int(cell.rowSpan); r++ {
			seen := false
			for col := cell.col - 1; col >= 0; col-- {
				if sc := scope(grid[r][col]); sc == "Both" || sc == "Row" {
					return true
				}
				if grid[r][col].std == "TH" {
					seen = true
				} else if seen {
					break
				}
			}
		}
	}
	return false
}

// TestTheHeaderReachTablesAgreeWithTheScanTheyReplaced — R4-2's semantics. The tabulation is a rewrite of a
// search whose every verdict was measured on veraPDF, so it is held to the old scan cell by cell, over 400
// seeded grids of TH and TD in every Scope, rather than to a handful of hand-picked tables.
func TestTheHeaderReachTablesAgreeWithTheScanTheyReplaced(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	scopes := []string{"", "!scope=Row", "!scope=Column", "!scope=Both"}
	compared, connected := 0, 0
	for trial := 0; trial < 400; trial++ {
		w, h := 1+rng.Intn(5), 1+rng.Intn(6)
		var rows []string
		for r := 0; r < h; r++ {
			var cells []string
			for c := 0; c < w; c++ {
				if rng.Intn(2) == 0 {
					cells = append(cells, "TD")
				} else {
					cells = append(cells, "TH"+scopes[rng.Intn(len(scopes))])
				}
			}
			rows = append(rows, "TR("+strings.Join(cells, ",")+")")
		}
		d, err := open(treeDoc("", "Document(Table("+strings.Join(rows, ",")+"))"))
		if err != nil {
			t.Fatal(err)
		}
		nodes, _ := d.structNodes()
		grid := make([][]*tableCell, h)
		i := 0
		for _, n := range nodes {
			std := d.typedAs(n.dict)
			if std != "TH" && std != "TD" {
				continue
			}
			r, c := i/w, i%w
			grid[r] = append(grid[r], &tableCell{dict: n.dict, std: std, row: r, col: c, rowSpan: 1, colSpan: 1})
			i++
		}
		if i != w*h {
			t.Fatalf("trial %d: the fixture laid out %d cells, want %d", trial, i, w*h)
		}
		reach := d.headerReachOf(grid)
		steps := 0
		for r := range grid {
			for _, cell := range grid[r] {
				want := bruteHeaderInLine(d, grid, cell)
				if got := reach.inLine(cell, &steps); got != want {
					t.Fatalf("trial %d: the cell at row %d, column %d is connected=%v by the tables and %v by the scan "+
						"(grid %s)", trial, r+1, cell.col+1, got, want, strings.Join(rows, ","))
				}
				compared++
				if want {
					connected++
				}
			}
		}
	}
	// The stimulus: both answers occurred, or agreement is agreement on one of them.
	if connected == 0 || connected == compared {
		t.Fatalf("%d of %d cells connected — the seeded grids exercised only one answer", connected, compared)
	}
}

// tallTable is one table of rows — a Column-scoped TH, an unscoped TH, then data cells — the reviewer's shape:
// every data cell's upward scan used to cross every data cell above it before meeting the headers.
func tallTable(rows int) []byte {
	var b strings.Builder
	b.WriteString("Document(Table(TR(TH!scope=Column),TR(TH)")
	for i := 2; i < rows; i++ {
		b.WriteString(",TR(TD)")
	}
	b.WriteString("))")
	return treeDoc("", b.String())
}

// TestTheHeaderSearchIsLinearInTheGrid — R4-2, measured 16,000 rows 0.79 s and 32,000 rows 3.08 s on one 7.5 t1
// call before the tabulation, 0.62 s and 0.93 s after (most of it now the tree walk, not the search). Asserted as
// WORK, not time: the slots the search read, against the slots the grid has.
func TestTheHeaderSearchIsLinearInTheGrid(t *testing.T) {
	const rows = 3000
	d, err := open(tallTable(rows))
	if err != nil {
		t.Fatal(err)
	}
	layouts, _, _ := d.tablesIn()
	if len(layouts) != 1 {
		t.Fatalf("%d tables laid out, want 1", len(layouts))
	}
	l := layouts[0]
	// The stimulus first: the search ran, over the whole grid.
	if l.headerSteps < rows {
		t.Fatalf("the header search read %d slots of a %d-slot grid — it did not run over the table this test is about",
			l.headerSteps, rows)
	}
	if got := checkTableHeaders(d); got.Verdict != Pass {
		t.Fatalf("7.5 t1 over the tall table = %v (%s), want Pass — every data cell sits under a Column-scoped header",
			got.Verdict, got.Why)
	}
	// Tabulating reads each slot once and each data cell asks one; a scan per cell reads ~rows²/2.
	if l.headerSteps > 3*rows {
		t.Errorf("the header search read %d slots of a %d-slot grid; it is linear in the grid or it is the "+
			"quadratic the P07 phase close measured", l.headerSteps, rows)
	}
}

// TestACellIsOfferedOnlyToTheLayoutsThatFlagIt — R4-3, measured 20,000 one-cell tables 4.5 s on one 7.2 t15 call
// before the index, 1.6 s after. Asserted as work: how many (layout, cell) pairs the verdict looked at.
func TestACellIsOfferedOnlyToTheLayoutsThatFlagIt(t *testing.T) {
	const tables = 2000
	var b strings.Builder
	b.WriteString("Document(")
	for i := 0; i < tables; i++ {
		b.WriteString("Table(TR(TD)),")
	}
	b.WriteString("Table(TR(TD,TD!r2),TR(TD!c2)))")
	d, err := open(treeDoc("", b.String()))
	if err != nil {
		t.Fatal(err)
	}
	intersect := map[string]bool{"TH": true, "TD": true}
	got, offered := d.cellVerdictCounted(intersect, func(l *tableLayout, id uintptr) string {
		if l.intersecting[id] {
			return "intersects"
		}
		return ""
	})
	// The stimulus first: the flagged cell was reached, past two thousand tables.
	if got.Verdict != Fail || offered == 0 {
		t.Fatalf("7.2 t15 = %v (%s) with %d pairs offered, want Fail — the intersecting table is the last one",
			got.Verdict, got.Why, offered)
	}
	if offered > 2 {
		t.Errorf("%d (layout, cell) pairs were offered for %d tables; only the one table that flagged a cell should "+
			"see any", offered, tables+1)
	}
	// And the index keeps a layout's order: the Pass over the same tables without the last one offers nothing.
	d2, err := open(treeDoc("", strings.TrimSuffix(b.String(), ",Table(TR(TD,TD!r2),TR(TD!c2)))")+")"))
	if err != nil {
		t.Fatal(err)
	}
	if res, n := d2.cellVerdictCounted(intersect, func(l *tableLayout, id uintptr) string { return "" }); res.Verdict != Pass || n != 0 {
		t.Errorf("over %d regular tables 7.2 t15 = %v with %d pairs offered, want Pass with none", tables, res.Verdict, n)
	}
}
