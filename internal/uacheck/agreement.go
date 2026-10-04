package uacheck

import (
	"fmt"
	"sort"
	"strings"
)

// knownDisagreements is every clause nib implements that is KNOWN to disagree with veraPDF on some document, with the
// item that owns the defect. Both corpus harnesses judge strictly — law 5's three states, so nib's Pass where veraPDF
// found no subject is a disagreement too — and fail on any disagreement not named (`producerDisagreements`,
// `corpusStrict`, `corpusAllow`); the oracle admits none. So this list is complete wherever the corpora exist, and
// `TestTheAgreementFigureIsTheHarnesssOwn` keeps it equal to the clauses the harnesses name.
var knownDisagreements = map[string]string{
	"7.1 t9":  "/pending 694",
	"7.2 t30": "/pending 674",
	"7.2 t31": "/pending 674",
	"7.2 t32": "/pending 674",
	"7.11 t1": "/pending 695",
}

// unexercised is every clause nib implements that no corpus has SETTLED (Pass or Fail) on any document — "no
// disagreement" there is an absence of evidence, not agreement, so it is not counted in N. Kept equal, by
// `TestTheAgreementFigureIsTheHarnesssOwn`, to the clauses neither veraPDF's corpus nor the oracle's generated documents
// settle. Empty since the P08 phase close: `7.18.2 t1`, which veraPDF's corpus never settles, is exercised in both
// directions by the oracle's two TrapNet documents.
var unexercised = map[string]string{}

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
// It claims only what was measured: the same verdict wherever a document exercises the rule. It does not claim every rule
// was exercised by every corpus — seven are settled on no real-producer file (the P08 phase-close review, R1-3).
func Agreement() string {
	// "wherever nib reached one" (/pending 727): every harness skips a CannotCheck rather than scoring it
	// (`veracorpus_test`, `producers_test`), so a document nib refused is not one it agreed on, and the sentence must
	// not read as though it were.
	s := fmt.Sprintf("nib agrees with veraPDF on %d of the %d rules it checks — the same verdict on every document that "+
		"exercises the rule, wherever nib reached one, in veraPDF's own PDF/UA-1 test corpus, in nib's generated test documents and in documents from "+
		"real producers; it is known to disagree on %d (%s)", AgreedClauses(), len(Clauses()),
		len(knownDisagreements), clauseList(knownDisagreements))
	if len(unexercised) > 0 {
		s += fmt.Sprintf(", and %d (%s) no document has yet exercised", len(unexercised), clauseList(unexercised))
	}
	return s
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
	f := []string{
		fmt.Sprintf("agrees with veraPDF on %d of the %d", AgreedClauses(), len(Clauses())),
		fmt.Sprintf("disagree on %d", len(knownDisagreements)),
	}
	if len(unexercised) > 0 {
		f = append(f, fmt.Sprintf("%d no document has yet exercised", len(unexercised)))
	}
	return f
}
