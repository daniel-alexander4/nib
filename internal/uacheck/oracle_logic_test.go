package uacheck

import (
	"strings"
	"testing"
)

// The guard's judgment, driven without veraPDF — `PLAN-accessibility.md` P07.S05.

// TestTheAgreementIsStrictInAllThreeStates — the whole point of reading `--passed`.
func TestTheAgreementIsStrictInAllThreeStates(t *testing.T) {
	for _, c := range []struct {
		vera veraState
		nib  Verdict
		want bool
	}{
		{veraFailed, Fail, true}, {veraPassed, Pass, true}, {veraNoSubject, NotApplicable, true},
		// The collapse S02 had to accept and S05 exists to refuse: a pass and a clause with no subject
		// are different findings, and scoring them alike certifies agreement nobody measured.
		{veraNoSubject, Pass, false}, {veraPassed, NotApplicable, false},
		{veraFailed, Pass, false}, {veraPassed, Fail, false}, {veraNoSubject, Fail, false}, {veraFailed, NotApplicable, false},
		// Law 4's third verdict is honest against anything — it is counted, not excused, elsewhere.
		{veraFailed, CannotCheck, true}, {veraPassed, CannotCheck, true}, {veraNoSubject, CannotCheck, true},
		// A rule that returned nothing never agrees.
		{veraPassed, NotRun, false},
	} {
		if got := c.vera.agrees(c.nib); got != c.want {
			t.Errorf("veraPDF %s against nib %v: agrees=%v, want %v", c.vera, c.nib, got, c.want)
		}
	}
}

// TestALostJobAndAnUnlistedClauseAreErrorsNotSilence — two failure paths the real corpus never takes.
func TestALostJobAndAnUnlistedClauseAreErrorsNotSilence(t *testing.T) {
	rep := &Report{Results: []Result{{Clause: "7.1 t11", Verdict: Pass}, {Clause: "7.1 t3", Verdict: Fail}}}
	cmp := compareToOracle(
		[]string{"a document veraPDF lost", "a document missing one clause", "a clean document"},
		[]map[string]veraState{
			nil,
			{"7.1 t11": veraPassed},
			{"7.1 t11": veraPassed, "7.1 t3": veraFailed},
		},
		[]*Report{rep, rep, rep},
	)
	joined := strings.Join(cmp.errors, "\n")
	if !strings.Contains(joined, "a document veraPDF lost: veraPDF returned no job") {
		t.Errorf("a lost job was not reported:\n%s", joined)
	}
	if !strings.Contains(joined, "a document missing one clause: veraPDF's report does not list 7.1 t3") {
		t.Errorf("an unlisted clause was not reported:\n%s", joined)
	}
	if len(cmp.errors) != 2 {
		t.Errorf("want exactly 2 errors (the clean document contributes none), got %d:\n%s", len(cmp.errors), joined)
	}
	if cmp.agreed != 3 || cmp.total != 4 {
		t.Errorf("agreed=%d total=%d, want 3 of 4 — the unlisted clause is counted and not agreed", cmp.agreed, cmp.total)
	}
}

// TestADisagreementAndACannotCheckAreEachAccountedFor.
func TestADisagreementAndACannotCheckAreEachAccountedFor(t *testing.T) {
	cmp := compareToOracle(
		[]string{"doc"},
		[]map[string]veraState{{"7.2 t33": veraNoSubject, "7.21.7 t1": veraFailed}},
		[]*Report{{Results: []Result{
			{Clause: "7.2 t33", Verdict: Fail, Why: "nib stricter than the clause"},
			{Clause: "7.21.7 t1", Verdict: CannotCheck, Why: "unmeasured font"},
		}}},
	)
	if len(cmp.errors) != 1 || !strings.Contains(cmp.errors[0], "7.2 t33 — veraPDF no subject, nib fail") {
		t.Errorf("the disagreement is not reported as one error naming both sides: %q", cmp.errors)
	}
	if why, ok := cmp.cannot["doc / 7.21.7 t1"]; !ok || why != "unmeasured font" {
		t.Errorf("the CannotCheck is not collected for the knownCannotCheck accounting: %v", cmp.cannot)
	}
	if !cmp.reached["7.2 t33 no subject"] || !cmp.reached["7.21.7 t1 failed"] {
		t.Errorf("reached states are not recorded: %v", cmp.reached)
	}
}
