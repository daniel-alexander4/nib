// Package uacheck is nib's own PDF/UA checker — `PLAN-accessibility.md` P07, D6.
//
// # Why nib has its own instead of shipping veraPDF
//
// D6, measured: veraPDF is a 2.32 s JVM invocation, and nib is a single cgo-free binary with no
// runtime dependencies. An in-app checker must therefore be nib's own code. It does not need to
// implement all of PDF/UA — it needs to implement what it can verify and to say plainly what it
// cannot (law 4), with agreement against veraPDF asserted over a corpus (law 5).
//
// veraPDF stays behind the test line, where it is the oracle rather than the product.
package uacheck

import (
	"fmt"
	"sort"
)

// Verdict is what one rule concluded about one document.
//
// # The zero value is `NotRun`, and that is the whole design
//
// Law 4: *"The checker never reports a pass it did not perform. Three verdicts — pass, fail, and
// cannot check, and why — and the third never collapses into the first."*
//
// A `Pass` at zero is how that law gets broken without anyone writing a line that breaks it: a rule
// that returns early, a map lookup that misses, a slice grown with `make`, a struct built by a test
// helper that forgets a field. Every one of those yields the zero value, and if the zero value says
// "conformant" then the document is reported conformant by an omission. So `NotRun` is 0, and it is
// not one of the three verdicts law 4 names — it is the state of never having been asked, which
// must be distinguishable from all three.
//
// This is the one thing a type can actually enforce here. A function can always `return Pass`
// having checked nothing; no type prevents that, and the acceptance clause that claimed otherwise
// was wrong. What the type CAN do is refuse to let absence look like conformance, and `Report` below
// refuses to call a document conformant while any rule is `NotRun`.
type Verdict int

const (
	// NotRun is the zero value: this rule has not been asked about this document. Never a pass.
	NotRun Verdict = iota
	// Pass — the rule was evaluated and the document satisfies it.
	Pass
	// Fail — the rule was evaluated and the document does not satisfy it.
	Fail
	// CannotCheck — the rule applies and nib is unable to evaluate it. Law 4's third verdict, and
	// the one that must never be read as a pass.
	CannotCheck
	// NotApplicable — the rule's subject is not present, so there is nothing to satisfy or breach.
	// It is distinct from `Pass` and from `CannotCheck`: `5 t1` over a document with no XMP packet
	// at all is not conformant-so-far and not a gap in nib, it is a clause with no subject. P03
	// measured exactly this — *"with no /Metadata stream at all, 5 t1 has no subject and is not
	// evaluated; an honest one makes it applicable."*
	NotApplicable
)

func (v Verdict) String() string {
	switch v {
	case NotRun:
		return "not run"
	case Pass:
		return "pass"
	case Fail:
		return "fail"
	case CannotCheck:
		return "cannot check"
	case NotApplicable:
		return "not applicable"
	}
	return fmt.Sprintf("Verdict(%d)", int(v))
}

// conformant says whether this verdict lets a document still be called conformant. **`NotRun` does
// not**, which is the law the zero value exists to serve.
func (v Verdict) conformant() bool { return v == Pass || v == NotApplicable }

// Result is one rule's conclusion, with what it concluded it FROM.
//
// `Why` is required for `Fail`, `CannotCheck` and `NotRun` and is checked — a verdict that cannot
// say why is the collapse law 4 forbids, arriving as an empty string instead of as a wrong enum.
// `Where` locates the subject: a page number, an object, a content-stream offset. P01 spent a slice
// discovering that veraPDF points at `xObject[0]/contentStream[0]/content[2]{mcid:0}`, and a
// checker that says only *"7.1 t3 fails"* reproduces the problem it exists to solve.
type Result struct {
	Clause  string  `json:"clause"` // "7.1 t3" — veraPDF's own spelling, so the two can be compared
	Verdict Verdict `json:"verdict"`
	Why     string  `json:"why,omitempty"`
	Where   string  `json:"where,omitempty"`
}

// Rule is one clause nib can ask about a document.
//
// **The clause string is veraPDF's own spelling** (`"7.1 t3"`), because law 5 compares nib's verdict
// to veraPDF's per clause and a translation table between two namings is a third thing to get wrong.
type Rule struct {
	Clause string
	// Summary is the clause in the specification's own words, kept short. It is what the report
	// shows a person, and quoting rather than paraphrasing is what lets a reader check nib's
	// reading against the standard.
	Summary string
	// Check evaluates the rule. It returns a verdict and the reason; returning `Pass` with a reason
	// is allowed and ignored, returning anything else without one is a defect the registry's guard
	// catches.
	Check func(d *Document) Result
}

// Report is every rule's conclusion about one document.
type Report struct {
	Results []Result `json:"results"`
}

// Conformant says whether the document satisfied every rule that was asked — every clause NIB
// implements — 101 of the 106 veraPDF evaluates since P07.S03's TrueType program (97 after P07.S02's per-glyph Unicode mapping, 96 after P07.S01's composite-font CMaps, 91 after P06.S05's form XObject semantic parent, 90 after P06.S04's Formula, 89 after P06.S03's encryption bit and reference XObjects, 87 after P06.S02's Suspects, embedded-file names and dynamic XFA, 84 after P06.S01's identification prefixes and file header, 80 after P05.S04's media clips, 78 after P05.S03's widget description and tab order, 76 after P05.S02's four typed annotation rules, 72 after P05.S01's annotation door, 70 after P04.S04's marked content, 65 after P04.S03's annotation and field text, 63 after P04.S02's alternate text, 60 after P04.S01's catalog language and identifiers, 58 after P03.S06's notes and forms, 55 after P03.S05's heading structure, 52 after P03.S04's table layout, 47 after P03.S03's cardinality and placement, 39 after P03.S02's containment matrix, 22 after P03.S01's 7.1 t5 and t7, 20 after /pending 548's 7.1 t6, 19 after /pending 487's 7.4.2 t1, 18 after /pending 489's 5 t2, 17 at P09.S05, 15 when P07.S07 measured it). It is not PDF/UA
// conformance, and nothing may treat it as licence to label a document (ADR-031 law 1).
//
// **A single `NotRun` or `CannotCheck` makes it false**, and that is law 4 in one line: a checker
// that has not finished cannot say a document conforms, and one that could not evaluate a clause
// has not established that the clause holds. Only `Pass` and `NotApplicable` count toward yes.
func (r Report) Conformant() bool {
	if len(r.Results) == 0 {
		// Nothing was asked. The honest answer to "is this conformant" is no — and an empty report
		// reading as conformant is the vacuous pass this whole package is arranged against.
		return false
	}
	for _, res := range r.Results {
		if !res.Verdict.conformant() {
			return false
		}
	}
	return true
}

// Unresolved returns the clauses that were not settled either way — `CannotCheck`, `NotRun`, and any value
// outside the enum.
//
// It exists so a caller cannot ask "did anything fail" and treat silence as conformance: the two
// questions are separate and this is the second one. **It is defined as the complement of the other two
// answers** — neither conformant nor `Fail` — rather than as a list of verdicts (`/pending 496`): listed,
// a `Verdict(42)` was neither failed nor unresolved, so `Refusals` came back empty for a report
// `Conformant` refused, and `nib ua` printed "every clause nib checks passes" over it.
func (r Report) Unresolved() []Result {
	var out []Result
	for _, res := range r.Results {
		if !res.Verdict.conformant() && res.Verdict != Fail {
			out = append(out, res)
		}
	}
	return out
}

// Failures returns the clauses the document breaches.
func (r Report) Failures() []Result {
	var out []Result
	for _, res := range r.Results {
		if res.Verdict == Fail {
			out = append(out, res)
		}
	}
	return out
}

// registry is every rule nib implements, keyed by clause.
//
// **Enumerated here and floored from OUTSIDE**, the shape `tagFates` uses: a rule that is never
// registered is a clause nobody checks, and it looks identical to a clause that passes. What floors it
// is `TestPassingEveryClauseNibChecksIsNotConformanceAndTheDocsSaySo`, which derives "N of the 106" and
// its complement from `len(Clauses())` and requires both to appear in the README, the parity doc and
// three prose sites — so an emptied map turns six files red. This comment used to claim a minimum-count
// assertion in the guard; there is none, and there never was (P05 phase close).
var registry = map[string]Rule{}

// register adds a rule, refusing a duplicate clause.
//
// A second rule for one clause is two opinions about the same question, and whichever `init` ran
// last would win silently.
func register(r Rule) {
	if r.Clause == "" || r.Check == nil {
		panic("uacheck: a rule needs a clause and a check")
	}
	if _, dup := registry[r.Clause]; dup {
		panic("uacheck: two rules registered for clause " + r.Clause)
	}
	registry[r.Clause] = r
}

// Clauses returns every registered clause, sorted.
func Clauses() []string {
	out := make([]string, 0, len(registry))
	for c := range registry {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Check runs every registered rule over pdf.
//
// **A rule that panics is reported as `CannotCheck`, not skipped.** A malformed document can make a
// reader panic deep inside pdfcpu — `handleOCR` already recovers one for exactly this reason — and
// a rule that vanished from the report would leave `Conformant()` reading the remaining rules as the
// whole story. Recovered, named, and counted as unresolved.
func Check(pdf []byte) (Report, error) {
	d, err := open(pdf)
	if err != nil {
		return Report{}, err
	}
	var rep Report
	for _, clause := range Clauses() {
		rule := registry[clause]
		rep.Results = append(rep.Results, runOne(rule, d))
	}
	return rep, nil
}

// runOne evaluates one rule and guarantees the result is well-formed whatever the rule did.
func runOne(rule Rule, d *Document) (res Result) {
	defer func() {
		if rec := recover(); rec != nil {
			res = Result{
				Clause:  rule.Clause,
				Verdict: CannotCheck,
				Why:     fmt.Sprintf("the check did not complete: %v", rec),
			}
		}
	}()
	res = rule.Check(d)
	// The rule may not rename the clause it was asked about, and it may not leave the verdict at
	// the zero value: a rule that returns a bare `Result{}` has answered nothing, and `NotRun` is
	// what that is — reported, never smoothed into a pass.
	res.Clause = rule.Clause
	if res.Verdict < NotRun || res.Verdict > NotApplicable {
		// A value outside the enum has answered nothing a reader can interpret; it is reported as what it is.
		res.Verdict, res.Why = NotRun, fmt.Sprintf("the rule returned verdict %d, which is none of the five", int(res.Verdict))
	}
	if res.Verdict == NotRun && res.Why == "" {
		res.Why = "the rule returned no verdict"
	}
	return res
}
