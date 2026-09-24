package uacheck

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// The door — `PLAN-accessibility.md` P07.S06.

// TestANonEmbeddedFontIsRefusedByNameAlongsideEveryOtherReason — the carried P04.S04 criterion, and
// why this phase is where it could finally move: the refusal names the font AND every other clause.
func TestANonEmbeddedFontIsRefusedByNameAlongsideEveryOtherReason(t *testing.T) {
	pdf, err := testpdf.Text("host")
	if err != nil {
		t.Fatal(err)
	}
	rep, refusals, err := CheckForUA(pdf)
	if err != nil {
		t.Fatal(err)
	}
	fails := rep.Failures()
	// Stimulus: a Courier page must fail more than the font clause, or "every OTHER reason" is
	// untested.
	if len(fails) < 2 {
		t.Fatalf("setup: the fixture fails %d clause(s); the criterion is about naming every one", len(fails))
	}
	joined := strings.Join(refusals, "\n")
	if !strings.Contains(joined, "7.21.4.1 t1 fails") || !strings.Contains(joined, "Courier") {
		t.Errorf("the refusal does not name the non-embedded font:\n%s", joined)
	}
	for _, f := range fails {
		if !strings.Contains(joined, f.Clause+" fails") {
			t.Errorf("clause %s fails and is missing from the refusal — a refusal naming only the font "+
				"is exactly what P04.S04 could not do better than:\n%s", f.Clause, joined)
		}
	}
	if len(refusals) != len(fails)+len(rep.Unresolved()) {
		t.Errorf("%d refusal(s) for %d failure(s) and %d unresolved clause(s)", len(refusals), len(fails), len(rep.Unresolved()))
	}
}

// TestAClauseNibCouldNotCheckIsARefusalNotASilence — law 4 through the door.
func TestAClauseNibCouldNotCheckIsARefusalNotASilence(t *testing.T) {
	rep := Report{Results: []Result{
		{Clause: "7.1 t11", Verdict: Pass},
		{Clause: "7.21.7 t1", Verdict: CannotCheck, Why: "an unmeasured font", Where: "page 1"},
	}}
	got := Refusals(rep)
	if len(got) != 1 || !strings.Contains(got[0], "7.21.7 t1 could not be checked: an unmeasured font (page 1)") {
		t.Errorf("a report whose only problem is an unchecked clause refuses with %q — labelling over it "+
			"would be law 4's collapse arriving through the export", got)
	}
}

// TestAConformantReportRefusesNothingAndAnEmptyOneRefuses.
func TestAConformantReportRefusesNothingAndAnEmptyOneRefuses(t *testing.T) {
	ok := Report{Results: []Result{{Clause: "7.1 t11", Verdict: Pass}, {Clause: "7.18.4 t1", Verdict: NotApplicable}}}
	if got := Refusals(ok); got != nil {
		t.Errorf("a conformant report refuses: %q", got)
	}
	if got := Refusals(Report{}); len(got) != 1 {
		t.Errorf("an empty report refuses %q — nothing checked must never read as nothing wrong", got)
	}
	// Failures come before the unresolved, so the first line a person reads is a thing to fix.
	mixed := Report{Results: []Result{
		{Clause: "7.2 t34", Verdict: CannotCheck, Why: "x"},
		{Clause: "7.1 t3", Verdict: Fail, Why: "y"},
	}}
	if got := Refusals(mixed); len(got) != 2 || !strings.HasPrefix(got[0], "7.1 t3 fails") {
		t.Errorf("refusals are not ordered failures-first: %q", got)
	}
}

// TestEveryRegisteredClauseHasASummaryTheReportCanShow.
func TestEveryRegisteredClauseHasASummaryTheReportCanShow(t *testing.T) {
	for _, c := range Clauses() {
		if SummaryOf(c) == "" {
			t.Errorf("clause %s has no summary for the report to show", c)
		}
	}
	if SummaryOf("9.9 t9") != "" {
		t.Error("an unregistered clause has a summary")
	}
}

// TestPassingEveryClauseNibChecksIsNotConformanceAndTheDocsSaySo — a standing reader over the claims nib
// makes in words.
//
// P07.S07's counterexample was a tagged Markdown document whose heading skipped a level: it passed all
// of nib's clauses and failed veraPDF's 7.4.2 t1. `/pending 487` made nib check that rule, and the class
// remains — a paragraph tagged `/Formula` with no alternate text passes all of them and fails 7.7 t1,
// held by `counterexample_test.go`. The code cannot close the class without implementing the whole
// profile, so what is asserted here is that nothing a person reads calls a passing report "PDF/UA".
func TestPassingEveryClauseNibChecksIsNotConformanceAndTheDocsSaySo(t *testing.T) {
	// The count is the registry's, never a literal: a literal here and a literal in the README agree with each
	// other and say nothing about the rule a 59th registration adds (P03's phase-close review).
	count := fmt.Sprintf("%d of the 106", len(Clauses()))
	for _, f := range []struct{ path, must string }{
		{"../../README.md", "not a PDF/UA certificate"},
		{"../../README.md", count},
		{"../../docs/accessibility-parity.md", count},
		{"../../web/index.html", "can still fail one it does not"},
		{"door.go", "nothing labels"},
		// **P04.S03 gives these two a reader, and the reason is that they had none and went stale
		// twice.** The seam inventory's P04.S02 row A9 recorded them as prose copies of the count with
		// no standing reader, found stale at 60 and corrected by hand; this slice moved the count again
		// and found them stale at 63. A number maintained by whoever remembers is a number that is
		// wrong, so it is read from the registry here like every other copy.
		{"rules_catalog.go", count},
		{"../pdfops/labelua.go", count},
		// **P04.S04 adds the last two, and they are why this list keeps growing.** Both were stale at
		// **60** — they had missed P04.S02's move to 63 AND S03's to 65, while the four copies above were
		// corrected by the slice that moved the count. Two slices' T05 steps each said "the two prose
		// copies" and each meant a different two. **Every copy of the CURRENT count now has a reader** — the
		// six rows above. Three other files say "of the 106" and are deliberately not here, because none of
		// them states the coverage figure: `uaid.go` says "only part of the 106" with no number,
		// `rules_structure.go` gives one rule's ordinal at the time it was added, and ADR-032/033 quote 19
		// as the figure when they were decided, which an ADR may not restate (STANDARDS §11, immutable).
		{"uacheck.go", count},
		{"door.go", count},
	} {
		b, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), f.must) {
			t.Errorf("%s no longer says %q — a passing report would read as a conformance verdict", f.path, f.must)
		}
	}
	readme, _ := os.ReadFile("../../README.md")
	if strings.Contains(string(readme), "when the document is not PDF/UA") {
		t.Error("the README says nib ua exits 1 \"when the document is not PDF/UA\", which makes exit 0 read as \"is PDF/UA\" — the overclaim P07.S07 removed")
	}
}
