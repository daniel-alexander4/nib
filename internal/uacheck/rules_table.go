package uacheck

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Table geometry and table headers — `PLAN-ua-coverage.md` P03.S04.
//
// # This is veraPDF's algorithm, ported, and the port is the point
//
// ua1 7.2 t15 and t41-t43, and 7.5 t1 and t2, are not profile expressions over the tree: the profile tests
// properties (`hasIntersection`, `numberOfRowWithWrongColumnSpan`, `wrongColumnSpan`, `hasConnectedHeader`,
// `unknownHeaders`) that veraPDF computes in Java, in `GFSETable.checkTable`
// (veraPDF-validation, `validation-model/.../gfse/GFSETable.java`). Measurement alone had not found the rule
// — `rules_semantic.go` said WHICH cell failed 7.5 t1 "followed no rule the measurement could state" — and
// the source states it: the table is laid out into a grid, the walk STOPS at the first irregularity, and
// only the first data cell without headers is flagged. So this file follows `checkTable` step for step, and
// the corpus, the in-repo oracle and the fixtures in `rules_table_test.go` are what check that it does.
//
//  1. Rows are the table's structurally significant `TR` kids, and those of its `THead`, `TBody` and
//     `TFoot`, in order (`getTR`). Pass-through tags are looked through (`passThrough`).
//  2. The width is the first row's cells' ColSpans summed; the height is each counted row's first cell's
//     RowSpan summed, skipping the rows a span covers (`getNumberOfColumns`, `getNumberOfRows`).
//  3. Cells are placed left to right into the next free slot of their row. The FIRST of these ends the
//     layout: a cell past the width (t42's row, no span named); a RowSpan past the height (t41's column); a
//     slot already taken (t15, both cells); a row with no cell (t43, width 0); after placement, a row with
//     empty slots (t43, its filled width).
//  4. Only a regular table goes on to headers. Every TH scoped → no data cell is asked. Otherwise each data
//     cell but the one at [0][0], at its own anchor, in grid order, must name `Headers` all of which are TH
//     `/ID`s; the FIRST that does not is flagged, t1 when it named none and t2 when it named unknown ones.
//     A cell with no valid Headers is still connected when a header it can see carries a Scope pointing at
//     it (`headerInLine`). **The source on veraPDF's integration branch disables that search for PDF/UA-1;
//     the installed 1.30.2 runs it**, measured: `7.5-t01-fail-c.pdf` has a TD directly under a `Column`-scoped
//     TH and a scopeless TH beside it, and 1.30.2 PASSES 7.5-1 there. The oracle is the version nib is
//     checked against, so the search is ported, with UA-1's default scope — none; only an explicit Scope counts.
//
// **Where veraPDF's Java would throw, nib does not guess.** A span of 0 or below is not an error there — it
// places nothing, and the row reads short (t43) — but a negative height sizes the grid negatively, and a row
// past the height veraPDF counted, or a negative ColSpan moving the next cell before column 0, indexes outside
// it. The layout records those as unlayable and every rule over that table answers `CannotCheck`.

// tableLayout is one table's grid, laid out once per document.
type tableLayout struct {
	unlayable                string           // why veraPDF's own layout would not complete, or ""
	wrongRow                 int              // numberOfRowWithWrongColumnSpan, or -1
	wrongColSpan             int              // wrongColumnSpan, or -1 when null
	wrongRowSpanCol          int              // numberOfColumnWithWrongRowSpan, or -1
	intersecting             map[uintptr]bool // cells (by dictionary identity, so inline cells count) flagged hasIntersection
	headerless               uintptr          // the one data cell flagged hasConnectedHeader=false, or 0
	unknownHeaders           bool             // whether that cell named headers no TH carries
	unscopedRow, unscopedCol int              // the first TH in grid order with no Scope, 1-based — the cell a person fixes
}

type tableCell struct {
	dict             types.Dict
	id               uintptr
	std              string
	rowSpan, colSpan int64 // Java longs, as veraPDF's getRowSpan/getColSpan return them
	row, col         int
}

// spanOf is a cell's RowSpan or ColSpan as veraPDF reads it: the first Table-owned INTEGER value, else 1.
func (d *Document) spanOf(cell types.Dict, key string) int64 {
	if v := d.tableAttributeOfType(cell, key, attrInteger); v != nil {
		n, _ := d.intValue(v)
		return int64(n)
	}
	return 1
}

// jint is Java's `int += long`: the sum truncated to 32 bits, which is what veraPDF's layout does to every
// running count (`numberOfRows`, `numberOfColumns`, `rowNumber`, `columnNumber`) while its comparisons stay in
// 64 bits. Porting it is what keeps a crafted span from sizing the grid in gigabytes: `/RowSpan 4294967297`
// wraps veraPDF's height to 1 (and fails 7.2 t41), where an untruncated port asked for 103 GB and the process
// died unrecoverably — found by P03.S04's review, measured under a ulimit.
func jint(a int, b int64) int { return int(int32(int64(a) + b)) }

// maxTableSlots bounds the grid nib allocates. veraPDF allocates whatever the wrapped counts say; a table
// past this is not one any real producer writes, and nib answers CannotCheck rather than risk the process.
const maxTableSlots = 1 << 22

// The value types veraPDF's attribute reader matches (`AttributeHelper.getAttributeValue`'s COSObjType).
// Each is asked through the Document's own typed readers, the one door for a typed value.
const (
	attrInteger = iota
	attrName
	attrArray
)

// hasAttrType reports whether v is of the attribute type veraPDF would accept. An integer is an INTEGER
// object — `2.0` is not, and `intValue` (pdfcpu's DereferenceInteger) refuses a real already.
func (d *Document) hasAttrType(v types.Object, kind int) bool {
	switch kind {
	case attrInteger:
		_, ok := d.intValue(v)
		return ok
	case attrName:
		// An EMPTY name is still a name — veraPDF's getNameAttributeValue returns "" for `/Scope /`, not null, so
		// such a TH counts as scoped. `d.name` answers "" for both absent and empty, so the type is asked instead.
		_, err := d.Ctx.XRefTable.DereferenceName(v, model.V10, nil)
		return err == nil
	case attrArray:
		a, err := d.Ctx.DereferenceArray(v)
		return err == nil && a != nil
	}
	return false
}

// tableAttributeOfType is veraPDF's `AttributeHelper.getAttributeValue`: the first attribute object owned by
// `/O /Table` whose key holds a value of the wanted type — in `/A` (one object, or an array of them), and only
// when `/A` has none, in the class map entries `/C` names, first class first. A value of another type is
// passed over, not returned: `/ColSpan 2.0` is no ColSpan, and the default applies.
func (d *Document) tableAttributeOfType(elem types.Dict, key string, kind int) types.Object {
	return d.attributeOfType(elem, "Table", key, kind)
}

// attributeOfType is `tableAttributeOfType` for any attribute owner — `/O /Table`, `/O /PrintField` (a Form's
// Role, 7.18.4 t2) — the one door for veraPDF's typed attribute reader.
func (d *Document) attributeOfType(elem types.Dict, owner, key string, kind int) types.Object {
	find := func(attr types.Object) types.Object {
		if attr == nil {
			return nil
		}
		o, err := d.Ctx.Dereference(attr)
		if err != nil || o == nil {
			return nil
		}
		objs := []types.Object{o}
		if arr, ok := o.(types.Array); ok {
			objs = arr
		}
		for _, x := range objs {
			ad := d.dict(x)
			if ad == nil || d.name(ad["O"]) != owner {
				continue
			}
			if v := ad[key]; v != nil && d.hasAttrType(v, kind) {
				return v
			}
		}
		return nil
	}
	if v := find(elem["A"]); v != nil {
		return v
	}
	root := d.dict(d.Catalog["StructTreeRoot"])
	if root == nil {
		return nil
	}
	classMap := d.dict(root["ClassMap"])
	if classMap == nil {
		return nil
	}
	c, err := d.Ctx.Dereference(elem["C"])
	if err != nil || c == nil {
		return nil
	}
	classes := []types.Object{c}
	if arr, ok := c.(types.Array); ok {
		classes = arr
	}
	for _, cl := range classes {
		name := d.name(cl)
		if name == "" {
			continue
		}
		if v := find(classMap[name]); v != nil {
			return v
		}
	}
	return nil
}

// layoutTable lays out one table, memoised by its object number (inline tables are laid out each time).
func (d *Document) layoutTable(table types.Dict, obj int) *tableLayout {
	if obj != 0 {
		if l, ok := d.tables[obj]; ok {
			return l
		}
	}
	l := d.computeLayout(table)
	if obj != 0 {
		if d.tables == nil {
			d.tables = map[int]*tableLayout{}
		}
		d.tables[obj] = l
	}
	return l
}

func (d *Document) computeLayout(table types.Dict) *tableLayout {
	l := &tableLayout{wrongRow: -1, wrongColSpan: -1, wrongRowSpanCol: -1, intersecting: map[uintptr]bool{}}
	// 1. rows (getTR)
	var rows [][]*tableCell
	cellsOf := func(tr types.Dict) []*tableCell {
		var out []*tableCell
		for _, k := range d.elementKids(tr) {
			std := d.typedAs(k)
			if std != "TH" && std != "TD" {
				continue
			}
			out = append(out, &tableCell{dict: k, id: dictID(k), std: std,
				rowSpan: d.spanOf(k, "RowSpan"), colSpan: d.spanOf(k, "ColSpan")})
		}
		return out
	}
	for _, k := range d.elementKids(table) {
		switch d.typedAs(k) {
		case "TR":
			rows = append(rows, cellsOf(k))
		case "THead", "TBody", "TFoot":
			for _, tr := range d.elementKids(k) {
				if d.typedAs(tr) == "TR" {
					rows = append(rows, cellsOf(tr))
				}
			}
		}
	}
	// 2. height and width (getNumberOfRows, getNumberOfColumns)
	height := 0
	for r := 0; r < len(rows); r++ {
		if r < 0 {
			l.unlayable = "a RowSpan large enough to wrap veraPDF's row count sends its layout to a row before the first, which it cannot read"
			return l
		}
		if len(rows[r]) == 0 {
			continue
		}
		rs := rows[r][0].rowSpan
		height = jint(height, rs)
		if rs > 1 {
			r = jint(r, rs-1)
		}
	}
	if height == 0 {
		return l
	}
	width := 0
	for _, c := range rows[0] {
		width = jint(width, c.colSpan)
	}
	if height < 0 || width < 0 {
		l.unlayable = fmt.Sprintf("a RowSpan or ColSpan below 1 gives the table a grid of %d × %d, which veraPDF's own layout cannot build", height, width)
		return l
	}
	if int64(height)*int64(width) > maxTableSlots {
		l.unlayable = fmt.Sprintf("the table's spans make a grid of %d × %d slots, more than nib lays out", height, width)
		return l
	}
	grid := make([][]*tableCell, height)
	for i := range grid {
		grid[i] = make([]*tableCell, width)
	}
	// c.row and c.col are ints; a span that passed the bounds checks is at most the grid's size.
	// 3. placement (checkRegular)
	regular := func() bool {
		for r, row := range rows {
			col := 0
			for _, c := range row {
				for col < width {
					if col < 0 {
						l.unlayable = "a negative ColSpan moves the next cell before the table's first column, which veraPDF's own layout cannot place"
						return false
					}
					if r >= height {
						l.unlayable = fmt.Sprintf("row %d lies past the %d rows veraPDF counts for this table, so its own layout cannot place it", r+1, height)
						return false
					}
					if grid[r][col] == nil {
						break
					}
					col++
				}
				if int64(col)+c.colSpan > int64(width) {
					l.wrongRow = r
					return false
				}
				if int64(r)+c.rowSpan > int64(height) {
					l.wrongRowSpanCol = col
					return false
				}
				c.row, c.col = r, col
				// Bounded by the two checks just above, so these loops stay inside the grid.
				for i := 0; i < int(c.rowSpan); i++ {
					for j := 0; j < int(c.colSpan); j++ {
						if o := grid[r+i][col+j]; o != nil {
							l.intersecting[c.id] = true
							l.intersecting[o.id] = true
							return false
						}
						grid[r+i][col+j] = c
					}
				}
				col = jint(col, c.colSpan)
			}
			if len(row) == 0 && width > 0 {
				l.wrongRow, l.wrongColSpan = r, 0
				return false
			}
		}
		for r := 0; r < height; r++ {
			empty := 0
			for c := 0; c < width; c++ {
				if grid[r][c] == nil {
					empty++
				}
			}
			if empty != 0 {
				l.wrongRow, l.wrongColSpan = r, width-empty
				return false
			}
		}
		return true
	}
	if !regular() {
		return l
	}
	// 4. headers (hasScope, hasHeaders — PDF/UA-1)
	ids := map[string]bool{}
	allScoped := true
	for r := 0; r < height; r++ {
		for c := 0; c < width; c++ {
			cell := grid[r][c]
			if cell.std != "TH" {
				continue
			}
			if id, ok := d.text(cell.dict["ID"]); ok && id != "" {
				ids[id] = true
			}
			if d.tableAttributeOfType(cell.dict, "Scope", attrName) == nil {
				if allScoped {
					l.unscopedRow, l.unscopedCol = r+1, c+1
				}
				allScoped = false
			}
		}
	}
	if allScoped {
		return l
	}
	for r := 0; r < height; r++ {
		for c := 0; c < width; c++ {
			cell := grid[r][c]
			if cell.std != "TD" || r != cell.row || c != cell.col || (r == 0 && c == 0) {
				continue
			}
			var headers []string
			if h := d.tableAttributeOfType(cell.dict, "Headers", attrArray); h != nil {
				arr, _ := d.Ctx.DereferenceArray(h)
				for _, e := range arr {
					if s, ok := d.text(e); ok {
						headers = append(headers, s)
					}
				}
			}
			connected := len(headers) > 0
			var unknowns []string
			for _, h := range headers {
				if !ids[h] {
					connected = false
					unknowns = append(unknowns, h)
				}
			}
			// veraPDF tests `unknownHeaders`, the unknown names JOINED with ",", against '' — so one unknown EMPTY
			// name joins to '' and reads as no unknown header at all (t1, not t2), where two join to ",". Ported as
			// written; found by P03.S04's review from the 1.30.2 bytecode.
			unknown := strings.Join(unknowns, ",") != ""
			if connected || d.headerInLine(grid, cell) {
				continue
			}
			l.headerless, l.unknownHeaders = cell.id, unknown
			return l
		}
	}
	return l
}

// dictID is a dictionary's identity. The tree walk and `elementKids` dereference the same object to the same
// map, so a cell found either way is the same cell — including one written inline, which has no object number.
func dictID(d types.Dict) uintptr { return reflect.ValueOf(d).Pointer() }

func init() {
	register(Rule{Clause: "7.2 t15", Summary: "a table cell shall not intersect another cell", Check: checkCellIntersection})
	register(Rule{Clause: "7.2 t41", Summary: "a table's columns shall span the same number of rows", Check: checkTableRowSpans})
	register(Rule{Clause: "7.2 t42", Summary: "a table's rows shall span the same number of columns", Check: checkTableOverflow})
	register(Rule{Clause: "7.2 t43", Summary: "a table's rows shall span the same number of columns (a row falls short)", Check: checkTableShortRow})
	register(Rule{Clause: "7.5 t1", Summary: "if a table's structure is not determinable via Headers and IDs, TH elements shall have a Scope attribute", Check: checkTableHeaders})
	register(Rule{Clause: "7.5 t2", Summary: "a TD's Headers shall name only IDs of TH elements in its table", Check: checkUnknownHeaders})
}

// tablesIn lays out every table in the document, a Table typed through the role map; it answers the layouts,
// the Table nodes they belong to, and why the tree was not fully read when it was not.
func (d *Document) tablesIn() ([]*tableLayout, []structNode, string) {
	nodes, unread := d.structNodes()
	var layouts []*tableLayout
	var tables []structNode
	for _, n := range nodes {
		if d.typedAs(n.dict) == "Table" {
			layouts = append(layouts, d.layoutTable(n.dict, n.obj))
			tables = append(tables, n)
		}
	}
	return layouts, tables, unread
}

// tableVerdict is the shape every Table-scoped rule here shares: the first table whose layout breaks the
// clause fails, an unlayable one is CannotCheck, and no table is NotApplicable.
func (d *Document) tableVerdict(broken func(l *tableLayout) string) Result {
	layouts, tables, unread := d.tablesIn()
	cannot := ""
	for i, l := range layouts {
		if why := broken(l); why != "" {
			return Result{Verdict: Fail, Why: why, Where: nodeWhere(tables[i], d.name(tables[i].dict["S"]))}
		}
		if l.unlayable != "" && cannot == "" {
			cannot = l.unlayable
		}
	}
	switch {
	case unread != "":
		return Result{Verdict: CannotCheck, Why: unread}
	case cannot != "":
		return Result{Verdict: CannotCheck, Why: cannot}
	case len(tables) == 0:
		return Result{Verdict: NotApplicable, Why: "the document has no Table element"}
	}
	return Result{Verdict: Pass}
}

func checkTableRowSpans(d *Document) Result {
	return d.tableVerdict(func(l *tableLayout) string {
		if l.wrongRowSpanCol < 0 {
			return ""
		}
		return fmt.Sprintf("a cell in column %d spans more rows than the table has", l.wrongRowSpanCol+1)
	})
}

func checkTableOverflow(d *Document) Result {
	return d.tableVerdict(func(l *tableLayout) string {
		if l.wrongRow < 0 || l.wrongColSpan >= 0 {
			return ""
		}
		return fmt.Sprintf("row %d's cells run past the %s the first row sets", l.wrongRow+1, "width")
	})
}

func checkTableShortRow(d *Document) Result {
	return d.tableVerdict(func(l *tableLayout) string {
		if l.wrongRow < 0 || l.wrongColSpan < 0 {
			return ""
		}
		return fmt.Sprintf("row %d spans %d column(s), and the first row spans more", l.wrongRow+1, l.wrongColSpan)
	})
}

// cellVerdict is the shape the cell-scoped rules share (veraPDF's SETableCell and SETD): the subjects are the
// document's cells of the given types, wherever they sit, and a cell fails only when its table's layout
// flagged it — so a cell outside any table passes, as veraPDF's null property does.
func (d *Document) cellVerdict(kinds map[string]bool, flagged func(l *tableLayout, id uintptr) string) Result {
	layouts, _, unread := d.tablesIn()
	nodes, _ := d.structNodes()
	subjects := 0
	for _, n := range nodes {
		if !kinds[d.typedAs(n.dict)] {
			continue
		}
		subjects++
		for _, l := range layouts {
			if why := flagged(l, dictID(n.dict)); why != "" {
				return Result{Verdict: Fail, Why: why, Where: nodeWhere(n, d.name(n.dict["S"]))}
			}
		}
	}
	cannot := ""
	for _, l := range layouts {
		if l.unlayable != "" {
			cannot = l.unlayable
			break
		}
	}
	switch {
	case unread != "":
		return Result{Verdict: CannotCheck, Why: unread}
	case cannot != "":
		return Result{Verdict: CannotCheck, Why: cannot}
	case subjects == 0:
		return Result{Verdict: NotApplicable, Why: "the document has no table cells of that kind"}
	}
	return Result{Verdict: Pass}
}

func checkCellIntersection(d *Document) Result {
	return d.cellVerdict(map[string]bool{"TH": true, "TD": true}, func(l *tableLayout, id uintptr) string {
		if l.intersecting[id] {
			return "this cell's row or column span covers a slot another cell already occupies"
		}
		return ""
	})
}

// checkTableHeaders evaluates ua1 7.5 t1: the one data cell veraPDF's walk stops at names no Headers.
func checkTableHeaders(d *Document) Result {
	return d.cellVerdict(map[string]bool{"TD": true}, func(l *tableLayout, id uintptr) string {
		if l.headerless == id && !l.unknownHeaders {
			// Named by the HEADER it turns on — the cell a person fixes — as the retired heuristic named it.
			return fmt.Sprintf("the header at row %d, cell %d has no Scope, and this data cell names no Headers, so nothing "+
				"connects it to a header — give every header cell a Scope (Row, Column or Both), or give this cell Headers "+
				"naming its headers' IDs", l.unscopedRow, l.unscopedCol)
		}
		return ""
	})
}

// checkUnknownHeaders evaluates ua1 7.5 t2: that cell names Headers, and one of them is no TH's ID.
func checkUnknownHeaders(d *Document) Result {
	return d.cellVerdict(map[string]bool{"TD": true}, func(l *tableLayout, id uintptr) string {
		if l.headerless == id && l.unknownHeaders {
			return "this data cell's Headers name an ID no header cell in its table carries"
		}
		return ""
	})
}

// headerInLine is veraPDF's geometric `hasHeaders` (PDF/UA-1, where a header's default scope is none): looking
// up each column the cell covers, and left along each row it covers, the nearest run of TH cells is read, and a
// TH whose explicit Scope is Both — or Column looking up, Row looking left — connects the cell. The run ends at
// the first non-TH cell after a TH, so a header beyond an intervening data cell is not the cell's header.
func (d *Document) headerInLine(grid [][]*tableCell, cell *tableCell) bool {
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
