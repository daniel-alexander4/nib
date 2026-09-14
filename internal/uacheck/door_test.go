package uacheck

import (
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

// TestPassingEveryClauseNibChecksIsNotConformanceAndTheDocsSaySo — P07.S07's measured counterexample,
// kept as a standing reader over the claims nib makes in words.
//
// A tagged Markdown document whose heading skips a level passes all of nib's clauses and fails
// veraPDF's 7.4.2 t1. The code cannot close that gap without implementing the whole profile, so what
// is asserted is that nothing a person reads calls a passing report "PDF/UA".
func TestPassingEveryClauseNibChecksIsNotConformanceAndTheDocsSaySo(t *testing.T) {
	for _, f := range []struct{ path, must string }{
		{"../../README.md", "not a PDF/UA certificate"},
		{"../../README.md", "15 of the 106"},
		{"../../web/index.html", "can still fail one it does not"},
		{"door.go", "nothing labels"},
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
