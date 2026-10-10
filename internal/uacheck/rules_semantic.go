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
// **`/ActualText` is a string OR A NAME, and nothing else** (`d.actualText`; `/pending 633`, measured): `/ActualText
// /a` passes and so does the empty name, while a number, a boolean, an array, a dictionary and `null` each FAIL as an
// absent key does. It counted as present whatever it held until those documents could be opened at all
// (`/pending 612`).
func hasAlternateText(d *Document, n structNode) bool {
	if _, ok := d.actualText(n.dict["ActualText"]); ok {
		return true
	}
	s, ok := d.text(n.dict["Alt"])
	return ok && s != ""
}

// checkFigureAlt evaluates ua1 7.3 t1: every Figure (through the role map) has a non-empty `/Alt` or an
// `/ActualText`.
func checkFigureAlt(d *Document) Result {
	return checkAlternateTextOn(d, "Figure",
		"a Figure has neither an alternate description (/Alt) nor replacement text (/ActualText), so a screen reader has nothing to say for it")
}

// checkAlternateTextOn is `7.3 t1` and `7.7 t1`'s one door (ADR-009): every element of type `typ` has
// alternate text, by `hasAlternateText`.
//
// **A definite failure beats a refusal** (the package's convention, `checkEmbeddedFileNames`): the scan
// runs over the elements nib DID read and type before either refusal is considered. Until the P06 phase
// close both clauses refused first — so a Formula with no alternate text that nib had already read went
// unreported whenever the tree ran past its bound, or some OTHER element could not be typed. (That second
// refusal is gone since `/pending 634`: an element that cannot be typed is simply not of this type.)
func checkAlternateTextOn(d *Document, typ, failWhy string) Result {
	nodes, unread := d.structNodes()
	found := 0
	for _, n := range nodes {
		// **An element on a role-map loop has no type, and so is not one of these** (`typedAs`; `/pending 634`).
		// Measured on veraPDF 1.30.2 over eleven documents: beside a loop a Figure with its `/Alt` PASSES 7.3 t1, one
		// without FAILS, and a document whose only element is on the loop has no subject. This rule used to refuse
		// the whole document there, on every file where 7.1 t6 already fails.
		if d.typedAs(n.dict) != typ {
			continue
		}
		found++
		if hasAlternateText(d, n) {
			continue
		}
		return Result{Verdict: Fail, Why: failWhy, Where: nodeWhere(n, d.name(n.dict["S"]))}
	}
	if unread != "" {
		return Result{Verdict: CannotCheck, Why: unread}
	}
	if found == 0 {
		return Result{Verdict: NotApplicable, Why: "the document has no " + typ + " structure elements"}
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
	return checkAlternateTextOn(d, "Formula",
		"a Formula has neither an alternate description (/Alt) nor replacement text "+
			"(/ActualText), so a screen reader reads the equation's glyphs rather than the equation")
}
