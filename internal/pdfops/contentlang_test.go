package pdfops

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestDeclaringAContentLanguageAddsNoUA1Clause — `PLAN-accessibility.md` P06.S04's third acceptance
// clause, measured on the ua1 oracle.
//
// # Why this test exists when the census already drives the operation
//
// `TestNoOperationAddsAUA1ClauseItsInputDidNotFail` drives every row of `tagFates`, this one
// included — and for this operation that reading is **vacuous**. The census fixture is a tagged
// document, every page of it is already marked, and `declareContentLang` skips a marked page by
// design. Measured: `bracketed=0`, output byte-identical to input. An operation that did not run
// adds no clause, and the census cannot tell that from an operation that ran cleanly.
//
// So the measurement has to happen on the population the operation is FOR: nib's own authored
// pages, which carry no marked content at all. The composed document (`AppendReadme`) differs from
// the same composition without this operation by exactly the bytes bracketed here, so this is the
// composed document's delta, taken where the veraPDF helper lives.
//
// # What it does NOT buy, measured 2026-09-11
//
// ua1 **7.2 t34** — natural language for page content — is cleared by the CATALOG `/Lang` and not
// by this declaration. Measured on this same fixture: catalog-only and catalog-plus-content fail
// the identical seven clauses, and content-only leaves 7.2 t34 failing. The mechanism is NOT
// measured and is not claimed here.
//
// That is the honest scope of this slice and not a defect in it. The case S04 is for is a fragment
// stapled into someone ELSE'S document, where the catalog belongs to that document and nib may not
// touch it — `pdfops.Append` keeps the first document's catalog whole. A declaration a conforming
// reader can find beats an English page declared German, whether or not a validator credits it.
//
// # How strong this differential is, stated rather than implied
//
// **Weak, by construction, and the construction is `alreadyMarked`.** This operation skips a page
// that already carries marked content, so it only ever runs on pages that are neither tagged nor
// artifacted — which is ua1 **7.1 t3**, and every document it can touch fails that before it
// starts. There is not much left to lose, and two probes confirm it: a mutation emitting an OC
// dict with no `/Name` added nothing (7.10 is checked against `/OCProperties`, not an inline
// operand) and one leaving the bracket UNCLOSED added nothing either.
//
// So what this test rules out is a clause this operation *adds*, on the population it actually
// has. It is not evidence that the declaration is well-formed — `TestTheComposedDocumentGainsNo…`
// validates, and the p2p tests read the bytes back. The one probe that does go red here is the
// stimulus floor, which is the failure that was actually available to make.
func TestDeclaringAContentLanguageAddsNoUA1Clause(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so P06.S04's third acceptance clause — the " +
			"composed document gains no ua1 clause — is UNCHECKED in this run.")
	}

	before, err := CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[` +
		`{"value":"About this document","anchor":"TopLeft","position":[72,720],` +
		`"font":{"name":"Helvetica","size":12}}]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	after, n, err := DeclareAuthoredProseLang(before)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus floor. Without it this test passes loudest on the no-op the census already gets.
	if n != 1 {
		t.Fatalf("setup: the declaration bracketed %d page(s), not 1 — this fixture is the "+
			"census's vacuous reading again, and the differential below would measure nothing", n)
	}

	dir := t.TempDir()
	bp, ap := filepath.Join(dir, "before.pdf"), filepath.Join(dir, "after.pdf")
	if err := os.WriteFile(bp, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ap, after, 0o600); err != nil {
		t.Fatal(err)
	}
	clauses := ua1FailedClauses(t, vp, []string{bp, ap})
	base, got := clauses["before.pdf"], clauses["after.pdf"]
	if base == nil || got == nil {
		t.Fatalf("veraPDF could not validate one of the pair (before=%v after=%v), so there is no "+
			"differential", base != nil, got != nil)
	}
	var added []string
	for c := range got {
		if !base[c] {
			added = append(added, c)
		}
	}
	sort.Strings(added)
	if len(added) > 0 {
		t.Errorf("declaring a content language adds ua1 clause(s) the page did not fail: %s\n\t"+
			"The page before fails %v. Marked content that claims no tagging must cost the "+
			"document nothing.", strings.Join(added, ", "), sortedClauses(base))
	}
	t.Logf("before fails %v; the declaration adds nothing", sortedClauses(base))
}
