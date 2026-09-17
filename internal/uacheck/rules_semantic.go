package uacheck

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The clauses the structure editor exists to satisfy — `PLAN-accessibility.md` P09.S05.
//
// P09's first exit criterion is a tree corrected end to end without leaving nib, and a person cannot
// correct what nib cannot show them is wrong. Missing alternate text on a figure and header cells with
// no scope are the two corrections the editor makes that no clause nib checked could see, so they are
// checked here, against veraPDF like every other rule (law 5).
//
// # 7.5 t1 answers only what veraPDF was measured to answer
//
// veraPDF's object for 7.5 t1 is every `TD`, and its test is "has a connected header, or names headers
// it cannot find"; whether a header is connected is decided by an algorithm over the table. Measured on
// the hand-built tables in `rules_semantic_test.go`, which a standing test re-measures:
//
//   - **A table whose every `TH` carries a Scope — or which has no `TH` — never failed**, wherever the
//     headers sat: above, below, before or after the cell, in either direction.
//   - **A table with an unscoped `TH` failed in every shape but one**, and WHICH cell failed followed no
//     rule the measurement could state: one cell of two identically placed ones, a different corner in
//     each shape, and nothing at all once a neighbouring cell named its `Headers`.
//
// So nib passes the first case and answers `CannotCheck` for the second, naming the header cell that
// has no Scope — which is what the person has to fix either way. A row or column span, and a `TD`
// outside any table, are `CannotCheck` for the same reason: the grid is veraPDF's, not nib's.

func init() {
	register(Rule{
		Clause:  "7.3 t1",
		Summary: "Figure tags shall include an alternative representation or replacement text",
		Check:   checkFigureAlt,
	})
	register(Rule{
		Clause:  "7.5 t1",
		Summary: "if a table's structure is not determinable via Headers and IDs, TH elements shall have a Scope attribute",
		Check:   checkTableHeaders,
	})
}

// nodeWhere names a structure element for a report.
func nodeWhere(n structNode, kind string) string {
	if n.obj == 0 {
		return fmt.Sprintf("an inline /%s structure element", kind)
	}
	return fmt.Sprintf("structure element %d 0 R (/%s)", n.obj, kind)
}

// checkFigureAlt evaluates ua1 7.3 t1: every Figure (through the role map) has a non-empty `/Alt` or an
// `/ActualText`.
func checkFigureAlt(d *Document) Result {
	nodes, unread := d.structNodes()
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	std, untyped := d.standardTypes(nodes)
	if untyped != "" {
		// An element nib cannot type may be the Figure, so "the document has no Figure" is not nib's to say.
		return Result{Verdict: CannotCheck, Why: untyped}
	}
	figures := 0
	for i, n := range nodes {
		if std[i] != "Figure" {
			continue
		}
		figures++
		if v, ok := n.dict["ActualText"]; ok && v != nil {
			continue
		}
		if v, ok := n.dict["Alt"]; ok && v != nil {
			if s, err := d.Ctx.DereferenceStringOrHexLiteral(v, model.V10, nil); err == nil && s != "" {
				continue
			}
		}
		return Result{
			Verdict: Fail,
			Why:     "a Figure has neither an alternate description (/Alt) nor replacement text (/ActualText), so a screen reader has nothing to say for it",
			Where:   nodeWhere(n, d.name(n.dict["S"])),
		}
	}
	if figures == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no Figure structure elements"}
	}
	return Result{Verdict: Pass}
}

// checkTableHeaders evaluates ua1 7.5 t1 as far as veraPDF's answer was measured — see the file header.
func checkTableHeaders(d *Document) Result {
	nodes, unread := d.structNodes()
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	std, untyped := d.standardTypes(nodes)
	if untyped != "" {
		// An element nib cannot type may be the Table, the row or the cell, so no grid here is settled.
		return Result{Verdict: CannotCheck, Why: untyped}
	}
	scoped := func(th types.Dict) bool {
		n := d.name(d.tableAttribute(th, "Scope"))
		return n == "Row" || n == "Column" || n == "Both"
	}
	namesHeaders := func(td types.Dict) bool {
		h, err := d.Ctx.DereferenceArray(d.tableAttribute(td, "Headers"))
		return err == nil && len(h) > 0
	}
	spanned := func(cell types.Dict) bool {
		for _, key := range []string{"RowSpan", "ColSpan"} {
			if v, ok := d.intValue(d.tableAttribute(cell, key)); ok && v > 1 {
				return true
			}
		}
		return false
	}

	dataCells, tables := 0, 0
	inTable := map[int]bool{}
	var unsettled, where string
	for ti := range nodes {
		if std[ti] != "Table" {
			continue
		}
		tables++
		// Rows are TR kids of the table, or of its THead, TBody and TFoot; cells are a row's TH and TD kids.
		var rows [][]int
		var addRows func(i, depth int)
		addRows = func(i, depth int) {
			if depth > maxWalkDepth {
				return
			}
			for _, k := range nodes[i].kids {
				switch std[k] {
				case "TR":
					var row []int
					for _, c := range nodes[k].kids {
						if st := std[c]; st == "TH" || st == "TD" {
							row = append(row, c)
							inTable[c] = true
						}
					}
					rows = append(rows, row)
				case "THead", "TBody", "TFoot":
					addRows(k, depth+1)
				}
			}
		}
		addRows(ti, 0)

		unscoped, spans, bare := "", false, false
		for r, row := range rows {
			for c, cell := range row {
				el := nodes[cell].dict
				if spanned(el) {
					spans = true
				}
				switch std[cell] {
				case "TH":
					if !scoped(el) && unscoped == "" {
						unscoped = fmt.Sprintf("row %d, cell %d", r+1, c+1)
					}
				case "TD":
					dataCells++
					if !namesHeaders(el) {
						bare = true
					}
				}
			}
		}
		switch {
		case unscoped != "" && bare:
			unsettled = "a header cell has no Scope, and not every data cell names its headers, so which cells it " +
				"heads is decided by an algorithm nib does not reproduce — give every header cell a Scope (Row, Column or Both)"
			where = fmt.Sprintf("table %d, header at %s", tables, unscoped)
		case spans && bare:
			unsettled = "the table has a row or column span, and the grid it makes is not one nib lays out"
			where = fmt.Sprintf("table %d", tables)
		}
	}
	for i, n := range nodes {
		if std[i] == "TD" && !inTable[i] {
			dataCells++
			unsettled = "a TD sits outside any Table's rows, so there is no grid to find its headers in"
			where = nodeWhere(n, "TD")
		}
	}
	switch {
	case unsettled != "":
		return Result{Verdict: CannotCheck, Why: unsettled, Where: where}
	case dataCells == 0:
		return Result{Verdict: NotApplicable, Why: "the document has no data cells (TD)"}
	}
	return Result{Verdict: Pass}
}
