package uacheck

import (
	"fmt"
	"sort"
	"strings"
)

// knownDisagreements is every clause nib implements that is KNOWN to disagree with veraPDF on some document, with the
// item that owns the defect. The real-producer harness (`producers_test.go`) judges strictly — law 5's three states —
// and fails on any disagreement not named; the veraPDF corpus test fails on any false pass or false fail not named. So
// this list is complete, under those two judgments, wherever the corpora exist; `TestTheAgreementFigureIsTheHarnesssOwn`
// keeps it equal to the clauses the two harnesses name.
var knownDisagreements = map[string]string{
	"7.1 t9":  "/pending 694",
	"7.2 t30": "/pending 674",
	"7.2 t31": "/pending 674",
	"7.2 t32": "/pending 674",
	"7.11 t1": "/pending 695",
}

// unexercised is every clause nib implements that no corpus has SETTLED (Pass or Fail) on any document — "no
// disagreement" there is an absence of evidence, not agreement, so it is not counted in N. Kept equal to veraPDF corpus
// reach's zero rows by `TestTheAgreementFigureIsTheHarnesssOwn`.
var unexercised = map[string]string{
	"7.18.2 t1": "veraPDF's corpus holds one TrapNet file, unreadable to pdfcpu, and no real producer draws a TrapNet",
}

// AgreedClauses is N in "agrees with veraPDF on N rules" (PLAN-ua-coverage.md law 2): the clauses nib checks that a
// corpus has exercised and on which no disagreement with veraPDF is known.
func AgreedClauses() int {
	n := 0
	for _, c := range Clauses() {
		_, off := knownDisagreements[c]
		_, idle := unexercised[c]
		if !off && !idle {
			n++
		}
	}
	return n
}

// Agreement is the one sentence that states N — `nib ua`'s note, and the figures the documentation is guarded against —
// so no surface can state a different figure or a stronger claim (law 2: "agrees with veraPDF", never "conformant").
// It says what each corpus measured: veraPDF's own is judged for false passes and false fails, the real-producer corpus
// for the same verdict.
func Agreement() string {
	return fmt.Sprintf("nib agrees with veraPDF on %d of the %d rules it checks — no false pass or false fail over "+
		"veraPDF's own PDF/UA-1 test corpus, and the same verdict on documents from real producers; it is known to "+
		"disagree on %d (%s), and %d (%s) no document has yet exercised", AgreedClauses(), len(Clauses()),
		len(knownDisagreements), clauseList(knownDisagreements), len(unexercised), clauseList(unexercised))
}

func clauseList(m map[string]string) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ", ")
}

// AgreementFigures are the phrases every document stating law 2's figure must carry, word for word — each number the
// sentence gives, so a disagreement that closes or a clause newly exercised cannot leave one of them stale in prose.
func AgreementFigures() []string {
	return []string{
		fmt.Sprintf("agrees with veraPDF on %d of the %d", AgreedClauses(), len(Clauses())),
		fmt.Sprintf("disagree on %d", len(knownDisagreements)),
		fmt.Sprintf("%d no document has yet exercised", len(unexercised)),
	}
}
