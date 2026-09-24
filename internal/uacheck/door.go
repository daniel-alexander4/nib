package uacheck

import (
	"fmt"
	"strings"
)

// The door both surfaces reach — `PLAN-accessibility.md` P07.S06.
//
// # Why one door and not a report each surface builds
//
// The UI and the CLI both answer "does every clause nib checks pass", and the plan's third acceptance
// clause is that they do not each decide what the verdicts mean. A server handler that
// called `Check` and composed its own refusal, beside a CLI command that did the same, would be two
// readings of law 4 that agree today — ADR-009's case. So both call `CheckForUA`, and a guard at the
// repo root refuses a direct `Check` call from either.
//
// # Why this door refuses and never labels
//
// The carried P04.S04 criterion is a REFUSAL: *"a document carrying non-embedded fonts is refused for
// UA export with the reason named."* Writing the PDF/UA identification was to be P07.S07's, and S07's
// measurement overturned it: nib checked 15 of the 106 rules veraPDF evaluates (87 of the 106 since P06.S02), and a
// Markdown heading that skipped a level passed all of them while failing veraPDF's 7.4.2 t1. That one is checked
// now; the class is not — a paragraph tagged `/Formula` with no alternate text passes every clause nib checks
// and fails 7.7 t1
// (`counterexample_test.go`). A door labelling on "every clause
// nib checks passes" would write a conformance assertion over a non-conformant document — ADR-031 law
// 1, by name. So nothing labels on this checker's say-so, and the word "conformant" in this package means only
// that. The one label nib writes is `pdfops.LabelUA`'s, on its own Markdown conversion, and it rests on
// veraPDF's measurement of that conversion, not on this report (ADR-033).

// CheckForUA checks pdf against every clause nib implements and returns the report with the reasons
// a checked clause stops it — one sentence per failed clause and per clause nib could not settle.
// An empty refusal list means every clause NIB CHECKS passes; it is not a PDF/UA verdict.
//
// **`refusals` is empty only when the report is conformant**, and a clause nib could not check is a
// refusal just as a failure is: law 4's third verdict never collapses into the first, and a door that
// labelled a document over an unchecked clause would be that collapse arriving through the export.
func CheckForUA(pdf []byte) (Report, []string, error) {
	rep, err := Check(pdf)
	if err != nil {
		return Report{}, nil, err
	}
	return rep, Refusals(rep), nil
}

// Refusals states, for each clause that stops a report being conformant, why — failures first, then
// the clauses nib could not settle. Nil for a conformant report.
//
// It is composed from `Failures` and `Unresolved` rather than from its own pass over the results, so
// the question "what stops conformance" has one reading: the same two methods a caller asking only
// "did anything fail" or "what is unsettled" gets.
func Refusals(rep Report) []string {
	if rep.Conformant() {
		return nil
	}
	if len(rep.Results) == 0 {
		return []string{"nothing was checked, so nothing can be said to conform"}
	}
	var out []string
	for _, r := range rep.Failures() {
		out = append(out, fmt.Sprintf("%s fails: %s%s", r.Clause, r.Why, wherePart(r)))
	}
	for _, r := range rep.Unresolved() {
		if r.Verdict == NotRun {
			out = append(out, fmt.Sprintf("%s was not run: %s", r.Clause, r.Why))
			continue
		}
		out = append(out, fmt.Sprintf("%s could not be checked: %s%s", r.Clause, r.Why, wherePart(r)))
	}
	return out
}

// wherePart renders a result's location for a refusal sentence.
func wherePart(r Result) string {
	if r.Where == "" {
		return ""
	}
	return " (" + r.Where + ")"
}

// SummaryOf returns the clause's text as the rule states it, or "" for a clause nib does not check.
func SummaryOf(clause string) string {
	return strings.TrimSpace(registry[clause].Summary)
}
