package uacheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryVeraPDFRunReadByVeraStatesAsksForPassedChecks — /pending 697. `veraStates` reads a 0/0 rule as "no subject"
// and tells a capped report by its job total exceeding the rules' sum, so it is only right over a report run with
// `--passed`: without it no passed rule is listed, the sum is 0, and every report would read capped — the
// "passed or no subject" that agrees with anything but a Fail, silently where it used to be a loud unlisted clause.
// Every function that runs veraPDF and reads the result through `veraStates` must ask for it.
func TestEveryVeraPDFRunReadByVeraStatesAsksForPassedChecks(t *testing.T) {
	files, _ := filepath.Glob("*_test.go")
	fset := token.NewFileSet()
	sites := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var reads bool
			var runs []*ast.CallExpr
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					reads = reads || fun.Name == "veraStates"
				case *ast.SelectorExpr:
					if x, ok := fun.X.(*ast.Ident); ok && x.Name == "exec" && fun.Sel.Name == "Command" {
						runs = append(runs, call)
					}
				}
				return true
			})
			if !reads {
				continue
			}
			for _, call := range runs {
				sites++
				text := string(src[fset.Position(call.Pos()).Offset:fset.Position(call.End()).Offset])
				if !strings.Contains(text, `"--passed"`) {
					t.Errorf("%s: %s runs veraPDF without --passed and reads it through veraStates: %s", f, fn.Name.Name, text)
				}
			}
		}
	}
	// A guard that found no site checks nothing: the oracle, the corpus batch, the 7.4 semantic test, the glyph door's
	// veraAsk and the capped re-run all read this way.
	if sites < 5 {
		t.Errorf("only %d veraPDF run(s) read through veraStates were found — the scan is reading nothing", sites)
	}
}

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
