package nib

import (
	"go/ast"
	"testing"
)

// Both surfaces reach one door — `PLAN-accessibility.md` P07.S06's third acceptance clause.
//
// ADR-009's guard shape: routing, not agreement. The UI (`internal/server`) and the CLI
// (`internal/cli`) must each call `uacheck.CheckForUA`, and nothing outside `internal/uacheck` may
// call `uacheck.Check` directly — a handler that did would compose its own refusal, and two
// readings of law 4 that agree today are what ADR-009 exists to prevent.
func TestTheUIAndTheCLIReachTheSameConformanceDoor(t *testing.T) {
	doorCalls := map[string]int{}
	provenanceCalls := map[string]int{}
	var bypasses []string
	// The module, not `internal/` alone, and by IMPORT NAME rather than the identifier `uacheck`
	// (/pending 808): `walkCallers` and `importNames` in tagdoor_test.go say why.
	scanned := walkCallers(t, "internal/uacheck", func(file, surface string, f *ast.File) {
		uacheck, dot := importNames(f, "nib/internal/uacheck")
		if dot {
			bypasses = append(bypasses, file+" (a dot import, which hides any uacheck.Check call from this guard)")
		}
		pdfops, _ := importNames(f, "nib/internal/pdfops")
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if ok && pdfops[pkg.Name] && sel.Sel.Name == "DescribeStructureSource" {
				provenanceCalls[surface]++
				return true
			}
			if !ok || !uacheck[pkg.Name] {
				return true
			}
			switch sel.Sel.Name {
			case "CheckForUA":
				doorCalls[surface]++
			case "Check":
				bypasses = append(bypasses, file)
			}
			return true
		})
	})
	if scanned < 100 {
		t.Fatalf("the scan read %d non-test file(s) — too few to be the tree", scanned)
	}
	for _, surface := range []string{"server", "cli"} {
		if doorCalls[surface] == 0 {
			t.Errorf("internal/%s does not call uacheck.CheckForUA, so that surface does not reach the "+
				"door the other one does — the UI and the CLI must not each decide what conformance means", surface)
		}
		// D4's provenance line has one door too, or the two surfaces describe one tree differently.
		if provenanceCalls[surface] == 0 {
			t.Errorf("internal/%s does not call pdfops.DescribeStructureSource, so its report does not "+
				"say which tier produced the structure, or says it in its own words", surface)
		}
	}
	for _, p := range bypasses {
		t.Errorf("%s calls uacheck.Check directly. Call uacheck.CheckForUA, so the refusal is the door's "+
			"and not a second reading composed here (ADR-009)", p)
	}
}
