package uacheck

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The clauses the structure editor exists to satisfy — `PLAN-accessibility.md` P09.S05.
//
// P09's first exit criterion is a tree corrected end to end without leaving nib, and a person cannot
// correct what nib cannot show them is wrong. Missing alternate text on a figure and header cells with
// no scope are the two corrections the editor makes that no clause nib checked could see, so they are
// checked here, against veraPDF like every other rule (law 5).
//
// # 7.5 t1 moved to `rules_table.go` (P03.S04)
//
// This file answered 7.5 t1 as far as veraPDF had been MEASURED to answer it, and said `CannotCheck` for
// a table with an unscoped header — "which cell failed followed no rule the measurement could state".
// veraPDF's own source states it (`GFSETable.checkTable`): the grid, then the FIRST data cell without
// headers. That algorithm is ported in `rules_table.go`, with 7.5 t2 beside it, and the measurements this
// comment described are the fixtures there.

func init() {
	register(Rule{
		Clause:  "7.3 t1",
		Summary: "Figure tags shall include an alternative representation or replacement text",
		Check:   checkFigureAlt,
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
