package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// D33's discharge, built at P07.S09a: **a guard that fails if either law figure is reachable from
// the tunable block.**
//
// # Why this and not the obvious test
//
// D33 says twice, in the decision and again in the plan-review pin that settled it, that this is
// what discharges the decision — and that the alternative, driving a value past each bound,
// "passes identically whichever file the constant lives in". That is the whole point. The
// difference between law and tuning is not observable at runtime: a program with `N = 8` in a
// tunable block and one with `N = 8` beside the structure behave the same, and every behavioural
// test passes against both. Only the source says which one you have.
//
// # The two law figures, and why they are law
//
// D33 as amended (2026-08-19, via /discuss, Dan): `N` (the candidate cap) and the punch ceiling
// are LAW; the ceremony-deadline maximum and the roster maximum are tunable. The reason is D6's
// pin — **an attacker supplies the candidates**, so a bound an operator can raise is not a bound.
//
// # What "reachable" means here, which the grill had to settle
//
// Not merely "declared in". `maxCandidatesPerSource` was a bare `8` inside the tunable block whose
// own comment said the eight came from `ceremony.MaxCandidates`. Nothing declared a law figure
// there and the law figure's VALUE was still an operator edit away, with a comment asserting an
// agreement that no mechanism kept. So the check is on the assignment: a constant in the tunable
// block that stands for a law figure must be written as a reference to it.

// tunableBlockFile is D16's constant block — the file whose whole premise is that its values are
// tuning. Named once here so a rename fails this guard loudly rather than making it scan nothing.
const tunableBlockFile = "clocks.go"

// lawFigures are D33's two, by the identifier each is declared under. The guard is about these
// two and not about every constant in the tree: `maxRaceCandidates` also lives in the tunable
// block and is also labelled law, but it is D16's figure and D33's discharge names its own.
var lawFigures = []string{"MaxCandidates", "punchBudgetPerSide"}

// TestNeitherLawFigureIsReachableFromTheTunableBlock is D33's discharge.
func TestNeitherLawFigureIsReachableFromTheTunableBlock(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, tunableBlockFile, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("cannot parse %s, so this guard scanned nothing: %v", tunableBlockFile, err)
	}

	// SETUP: the block really is there and really holds constants. A guard that parses an empty
	// or renamed file reports "no law figure found here" for the wrong reason — which is the
	// vacuous green this repo keeps finding, applied to a source scan.
	consts := map[string]ast.Expr{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i < len(vs.Values) {
					consts[name.Name] = vs.Values[i]
				}
			}
		}
	}
	if len(consts) < 5 {
		t.Fatalf("%s declares %d constant(s); this guard is reading the wrong file or the "+
			"tunable block has moved, and either way its clean result means nothing",
			tunableBlockFile, len(consts))
	}

	// 1. Neither law figure is DECLARED here.
	for _, law := range lawFigures {
		if _, ok := consts[law]; ok {
			t.Errorf("%s declares %s. That figure is LAW (D33 as amended): under D6's pin an "+
				"attacker supplies the candidates, and a bound sitting in the block whose own "+
				"premise is that its values are tuning is a bound an operator may raise.",
				tunableBlockFile, law)
		}
	}

	// 2. `maxCandidatesPerSource` is a REFERENCE, not a literal. This is the instance the guard
	//    was written from: the constant stands for a law figure, so writing it as a number puts
	//    that figure's value inside the tunable block however carefully the comment explains it.
	per, ok := consts["maxCandidatesPerSource"]
	if !ok {
		t.Fatal("maxCandidatesPerSource is no longer declared in the tunable block; this guard " +
			"is asserting a property of a constant that has moved")
	}
	if lit, isLit := per.(*ast.BasicLit); isLit {
		t.Errorf("maxCandidatesPerSource is the literal %s. Its own comment says the value comes "+
			"from `ceremony.MaxCandidates` and `maxLANCandidates` — so a law figure is reachable "+
			"from the tunable block by hand-copy, which is D33's discharge condition verbatim. "+
			"Raise the upstream bound and this silently stays behind, capping an honest source "+
			"below what its own record is allowed. Derive it.", lit.Value)
	}
	// And the reference names the law figure rather than some other number that happens to match.
	//
	// `exprText` is `quoterect_test.go`'s, reused rather than reimplemented — P07.S06 wrote it
	// for the guard over `NominalBlockRect`, which exists because a rule had two implementations.
	// A second copy of the helper that catches copies would have been the joke writing itself.
	if !strings.Contains(exprText(fset, per), "MaxCandidates") {
		t.Errorf("maxCandidatesPerSource is %q, which does not read the law figure it claims to "+
			"track. A derivation from something else is a second hand-copy with an extra step.",
			exprText(fset, per))
	}
}

// TestTheLawFiguresLiveWithTheStructureTheyBound is the other half, and it fails differently:
// the check above says the law is not in the tunable block, this one says it is somewhere.
//
// Without it, deleting a law figure entirely passes the first check — nothing is reachable from
// the tunable block when nothing exists — which is the shape of green this repo files as vacuous.
func TestTheLawFiguresLiveWithTheStructureTheyBound(t *testing.T) {
	homes := map[string]string{
		"MaxCandidates":      "../ceremony/candidate.go",
		"punchBudgetPerSide": "punch.go",
	}
	for name, path := range homes {
		// PARSED, not grepped, and the red proof is why. The first version matched the substring
		// `name + " = "`, which a perfectly ordinary typed declaration — `punchBudgetPerSide int
		// = 3000` — does not contain. The guard would have failed a legal move it has no opinion
		// about, and a guard that cries at correct code is one somebody eventually silences.
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Errorf("%s's home %s cannot be parsed: %v", name, path, err)
			continue
		}
		if !declaresConst(f, name) {
			t.Errorf("%s is not declared in %s. D33's split puts it with the STRUCTURE it "+
				"bounds; a figure that has moved has either found a new home this guard does "+
				"not know about, or found the tunable block.", name, path)
		}
	}
}

// declaresConst reports whether a file declares a constant of that name.
func declaresConst(f *ast.File, name string) bool {
	found := false
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				if n.Name == name {
					found = true
				}
			}
		}
	}
	return found
}
