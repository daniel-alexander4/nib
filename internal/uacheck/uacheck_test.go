package uacheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The checker's spine and law 4 — `PLAN-accessibility.md` P07.S01.

// TestTheZeroVerdictIsNotAPass — law 4's collapse, made impossible to reach by omission.
//
// # Why this is the test that matters most in the package
//
// Law 4: *"The checker never reports a pass it did not perform … the third never collapses into the
// first."* Nothing stops a rule from writing `return Result{Verdict: Pass}` having checked nothing —
// no type can. What a type CAN stop is ABSENCE reading as conformance, and absence is how this law
// actually gets broken: a rule that returns early, a map lookup that misses, a `make([]Result, n)`,
// a test helper that forgets a field. Every one of those produces the zero value.
//
// So the assertion is on the zero value itself, in three places at once, because they are three
// separate ways for it to leak.
func TestTheZeroVerdictIsNotAPass(t *testing.T) {
	var v Verdict
	if v == Pass {
		t.Error("the zero Verdict IS Pass. A rule that returns early, a map that misses, or a " +
			"slice grown with make now reports a document conformant by omission — which is law 4 " +
			"broken without a single line being written that breaks it")
	}
	if v != NotRun {
		t.Errorf("the zero Verdict is %v, want NotRun — the state of never having been asked has "+
			"to be one of the values, or it borrows the meaning of whichever is 0", v)
	}
	if v.conformant() {
		t.Error("the zero Verdict counts toward conformance")
	}
	// A report built from zero values is not conformant.
	rep := Report{Results: make([]Result, 3)}
	if rep.Conformant() {
		t.Error("a report of three unasked rules reports the document CONFORMANT. This is the " +
			"vacuous pass in its purest form: nothing was checked and the answer was yes")
	}
	if n := len(rep.Unresolved()); n != 3 {
		t.Errorf("%d of 3 unasked rules are reported unresolved, want 3 — a rule that was never "+
			"run and one that passed must not be indistinguishable to a caller", n)
	}
}

// TestAnEmptyReportIsNotConformant — the other end of the same law.
//
// A checker that has no rules, or that was asked nothing, cannot say a document conforms. Reported
// separately from the zero-verdict case because the two fail differently: this one has no results
// at all rather than results that mean nothing.
func TestAnEmptyReportIsNotConformant(t *testing.T) {
	if (Report{}).Conformant() {
		t.Error("an empty report says the document is conformant. Nothing was asked, so the " +
			"honest answer is no — and a checker with a broken registry would otherwise pass " +
			"every document it was handed")
	}
}

// TestCannotCheckNeverCountsAsAPass — law 4's third verdict, which is the one it names.
func TestCannotCheckNeverCountsAsAPass(t *testing.T) {
	for _, v := range []Verdict{CannotCheck, NotRun} {
		if v.conformant() {
			t.Errorf("%v counts toward conformance. Law 4: the third verdict never collapses into "+
				"the first — a clause nib could not evaluate has not been established to hold", v)
		}
	}
	// And the two that DO count, asserted so this test cannot pass by rating everything unresolved.
	for _, v := range []Verdict{Pass, NotApplicable} {
		if !v.conformant() {
			t.Errorf("%v does NOT count toward conformance, so no document could ever be "+
				"conformant and every assertion above holds trivially", v)
		}
	}
	rep := Report{Results: []Result{
		{Clause: "7.1 t11", Verdict: Pass},
		{Clause: "7.1 t3", Verdict: CannotCheck, Why: "nib cannot read this content stream"},
	}}
	if rep.Conformant() {
		t.Error("a report with one pass and one cannot-check says CONFORMANT")
	}
	if len(rep.Failures()) != 0 {
		t.Error("a cannot-check is being reported as a failure — it is neither, and conflating " +
			"them tells the user their document breaches a clause nib never evaluated")
	}
	if len(rep.Unresolved()) != 1 {
		t.Error("the cannot-check is not reported as unresolved, so a caller asking `did anything " +
			"fail` gets no and treats silence as conformance")
	}
}

// TestNotApplicableIsNeitherAPassNorAGap — the distinction P03 measured.
//
// `5 t1` over a document with no XMP packet at all has no subject: P03 recorded that *"with no
// /Metadata stream at all, 5 t1 has no subject and is not evaluated; an honest one makes it
// applicable."* That is not a gap in nib (so not `CannotCheck`) and not the document satisfying
// something (so distinct from `Pass` in the report), and having a name for it is what keeps the
// other two honest.
func TestNotApplicableIsNeitherAPassNorAGap(t *testing.T) {
	if NotApplicable == Pass || NotApplicable == CannotCheck {
		t.Fatal("NotApplicable is not a distinct verdict")
	}
	if !NotApplicable.conformant() {
		t.Error("a clause with no subject blocks conformance, so a document that simply has no " +
			"forms could never be conformant")
	}
	rep := Report{Results: []Result{{Clause: "7.18.4 t1", Verdict: NotApplicable, Why: "the document has no widget annotations"}}}
	if len(rep.Unresolved()) != 0 {
		t.Error("a not-applicable clause is being counted as unresolved, which would make every " +
			"document without a form look like a checker gap")
	}
	if !rep.Conformant() {
		t.Error("a document whose only clause has no subject is reported non-conformant")
	}
}

// TestEveryRegisteredRuleIsWellFormed — the registry, floored the way `tagFates` is.
//
// A rule that is never registered is a clause nobody checks, and it looks exactly like a clause that
// passes. The floor is what makes an emptied registry visible; P07.S02–S04 raise it as they land.
func TestEveryRegisteredRuleIsWellFormed(t *testing.T) {
	clauses := Clauses()
	// Raised as rules land: S01 registered 1, S02 brought it to 9, S03 to 12, S04 to 15. A registry that
	// shrinks below what has shipped is a clause silently dropped, and it reads exactly like one
	// that passes.
	if len(clauses) < 15 {
		t.Fatalf("the registry holds %d rule(s); P07.S04 shipped 15. A rule dropped from the "+
			"registry is a clause nobody checks and looks identical to one that passes", len(clauses))
	}
	for _, c := range clauses {
		r := registry[c]
		if r.Clause != c {
			t.Errorf("rule registered under %q reports its clause as %q", c, r.Clause)
		}
		if r.Check == nil {
			t.Errorf("rule %q has no check", c)
		}
		if r.Summary == "" {
			t.Errorf("rule %q has no summary. The report shows it to a person, and quoting the "+
				"clause rather than paraphrasing is what lets a reader check nib's reading "+
				"against the standard", c)
		}
		// veraPDF's own spelling, because law 5 compares per clause and a translation table
		// between two namings is a third thing to get wrong.
		if !strings.Contains(c, " t") {
			t.Errorf("clause %q is not in veraPDF's `<clause> t<test>` spelling, so law 5's "+
				"agreement guard cannot line it up against the oracle", c)
		}
	}
}

// TestAVerdictThatCannotSayWhyIsReported — a rule answering nothing must not vanish.
//
// The failure mode is a rule returning a bare `Result{}`: the clause it was asked about is right
// there in the registry, so the report can still name it, and `runOne` fills the reason. A result
// that went missing instead would leave `Conformant()` reading the remaining rules as the whole
// story.
func TestAVerdictThatCannotSayWhyIsReported(t *testing.T) {
	res := runOne(Rule{
		Clause:  "9.9 t9",
		Summary: "a rule that answers nothing",
		Check:   func(*Document) Result { return Result{} },
	}, nil)
	if res.Clause != "9.9 t9" {
		t.Errorf("the result lost the clause it was asked about: %+v", res)
	}
	if res.Verdict != NotRun {
		t.Errorf("a rule that returned a bare Result{} reports %v", res.Verdict)
	}
	if res.Why == "" {
		t.Error("a NotRun verdict carries no reason, so the report says a clause was not checked " +
			"and cannot say why — which is the collapse law 4 forbids arriving as an empty string " +
			"rather than as a wrong verdict")
	}
}

// TestARuleThatPanicsBecomesCannotCheckRatherThanVanishing.
//
// A malformed document can make a reader panic deep inside pdfcpu — `handleOCR` already recovers one
// for exactly that reason. A rule that disappeared from the report would leave `Conformant()`
// reading the surviving rules as the whole story, which is a pass the checker did not perform.
func TestARuleThatPanicsBecomesCannotCheckRatherThanVanishing(t *testing.T) {
	res := runOne(Rule{
		Clause:  "9.9 t8",
		Summary: "a rule that panics",
		Check:   func(*Document) Result { panic("deep inside a reader") },
	}, nil)
	if res.Verdict != CannotCheck {
		t.Errorf("a panicking rule reports %v, want CannotCheck", res.Verdict)
	}
	if !strings.Contains(res.Why, "deep inside a reader") {
		t.Errorf("the recovered reason does not name what happened: %q", res.Why)
	}
	rep := Report{Results: []Result{{Clause: "7.1 t11", Verdict: Pass}, res}}
	if rep.Conformant() {
		t.Error("a document one of whose rules panicked is reported CONFORMANT")
	}
}

// TestEveryRuleWrittenInTheSourceIsActuallyRegistered — the `tagFates` enumeration shape, which
// S01's acceptance asks for by name.
//
// # What a map-and-init cannot tell you
//
// `Clauses()` reports what the registry HOLDS. It cannot report a rule that was written and never
// reached: a `register` call in a function nothing calls, an `init` in a file excluded by a build
// tag, a rule constructed into a variable and dropped. Each of those is a clause nobody checks, and
// to `Clauses()` it is indistinguishable from a clause that does not exist.
//
// So the population comes from the SOURCE — every `Clause:` literal passed to `register` — and is
// compared against what the registry ended up with. That is `tagFates`' own idiom one package over:
// enumerate from the code, never from a hand-written list.
func TestEveryRuleWrittenInTheSourceIsActuallyRegistered(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	inSource := map[string]string{} // clause -> file
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", name, perr)
		}
		scanned++
		ast.Inspect(f, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != "register" {
				return true
			}
			for _, arg := range call.Args {
				lit, isLit := arg.(*ast.CompositeLit)
				if !isLit {
					continue
				}
				for _, el := range lit.Elts {
					kv, isKV := el.(*ast.KeyValueExpr)
					if !isKV {
						continue
					}
					if k, ok := kv.Key.(*ast.Ident); !ok || k.Name != "Clause" {
						continue
					}
					if v, ok := kv.Value.(*ast.BasicLit); ok && v.Kind == token.STRING {
						inSource[strings.Trim(v.Value, `"`)] = name
					}
				}
			}
			return true
		})
	}

	// **Stimulus floor, both halves.** A scan that parsed nothing reports no rules in the source and
	// agrees with any registry; a registry that is empty agrees with a source that has none.
	if scanned < 2 {
		t.Fatalf("the scan read %d non-test file(s) in this package — too few to be it", scanned)
	}
	if len(inSource) == 0 {
		t.Fatal("the scan found no `register(Rule{Clause: …})` call in the package source. Either " +
			"registration is spelled differently now or the matcher has stopped working, and a " +
			"clean report is indistinguishable from a broken scan")
	}

	registered := map[string]bool{}
	for _, c := range Clauses() {
		registered[c] = true
	}
	for clause, file := range inSource {
		if !registered[clause] {
			t.Errorf("%s writes a rule for clause %q and the registry does not hold it — the rule "+
				"was never reached, so that clause is unchecked and looks identical to one that "+
				"does not exist", file, clause)
		}
	}
	for clause := range registered {
		if _, inSrc := inSource[clause]; !inSrc {
			t.Errorf("the registry holds %q and no `register` call in the source names it, so the "+
				"population this guard reads is not the one the package actually builds", clause)
		}
	}
}
