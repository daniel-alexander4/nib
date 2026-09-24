package uacheck

import (
	"fmt"
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
	register(Rule{
		Clause:  "7.7 t1",
		Summary: "Formula tags shall include an alternative representation or replacement text",
		Check:   checkFormulaAlt,
	})
}

// nodeWhere names a structure element for a report.
func nodeWhere(n structNode, kind string) string {
	if n.obj == 0 {
		return fmt.Sprintf("an inline /%s structure element", kind)
	}
	return fmt.Sprintf("structure element %d 0 R (/%s)", n.obj, kind)
}

// hasAlternateText is the predicate `7.3 t1` and `7.7 t1` share — the ONE door (ADR-009), since both
// clauses are the same profile test over a different structure type:
//
//	(Alt != null && Alt != '') || ActualText != null
//
// **The asymmetry is the profile's, and it is measured rather than tidied**: an EMPTY `/Alt` FAILS,
// because the test requires it to be non-empty, while an EMPTY `/ActualText` PASSES, because the test
// only requires it to be present. Making the two consistent would be nib disagreeing with the oracle
// on a document veraPDF accepts.
//
// **`/ActualText` is RESOLVED, not merely present.** A raw map read returns true for a reference to an
// object that does not exist, and veraPDF reads such a value as ABSENT — measured: a Formula whose
// only alternate text is `/ActualText 9999 0 R` with object 9999 missing was PASSED by nib and FAILED
// by veraPDF, while the same dangling reference on `/Alt` was failed by both, because that arm already
// went through `d.text`. One half of this door had been corrected at P04.S02 (`document.go` records
// the measurement) and the other had not; extracting the predicate is what put them side by side.
//
// **`/ActualText` of a non-string type is still accepted here, and that is `/pending 633`** — a
// different question, about what veraPDF does with a value that is present but is not a string. It is
// not the dangling-reference case above, which is an ABSENT value.
func hasAlternateText(d *Document, n structNode) bool {
	if v, ok := n.dict["ActualText"]; ok && d.resolve(v) != nil {
		return true
	}
	s, ok := d.text(n.dict["Alt"])
	return ok && s != ""
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
		if hasAlternateText(d, n) {
			continue
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

// checkFormulaAlt evaluates ua1 7.7 t1: every Formula (through the role map) has a non-empty `/Alt` or
// an `/ActualText` (P06.S04).
//
// # This rule RETIRES the counterexample this repo's own documents cite
//
// Until it shipped, SIX sites named the same document as the proof that passing every clause nib checks
// is not conformance — `README.md`, `door.go`, `internal/cli/commands.go`, `internal/cli/cli_test.go`,
// `door_test.go` and `PLAN-accessibility.md`: *"a paragraph tagged as a formula with no alternate text
// passes all of Nib's checks and fails veraPDF"*. **The phase open said FOUR and named two files that
// never carried it** (`rules_catalog.go`, `docs/accessibility-parity.md` — they carry the COUNT, not
// the example), which is how three real ones were missed on the first pass and found by review.
// ADR-031 law 1 rests on such a document EXISTING, and `counterexample_test.go` built and asserted
// exactly that one — designed to go red the day a rule caught it. That day is this slice, so the
// example moved rather than the test being weakened, and the new one is measured the same way.
//
// A Formula is a mathematical expression rendered as marked-up content or as a picture; without an
// alternate description a screen reader reads whatever glyphs happen to be there, which for an
// equation is noise.
func checkFormulaAlt(d *Document) Result {
	nodes, unread := d.structNodes()
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	std, untyped := d.standardTypes(nodes)
	if untyped != "" {
		// An element nib cannot type may be the Formula, so "the document has no Formula" is not
		// nib's to say — the same refusal `7.3 t1` makes for the same reason.
		return Result{Verdict: CannotCheck, Why: untyped}
	}
	formulas := 0
	for i, n := range nodes {
		if std[i] != "Formula" {
			continue
		}
		formulas++
		if hasAlternateText(d, n) {
			continue
		}
		return Result{
			Verdict: Fail,
			Why: "a Formula has neither an alternate description (/Alt) nor replacement text " +
				"(/ActualText), so a screen reader reads the equation's glyphs rather than the equation",
			Where: nodeWhere(n, d.name(n.dict["S"])),
		}
	}
	if formulas == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no Formula structure elements"}
	}
	return Result{Verdict: Pass}
}
