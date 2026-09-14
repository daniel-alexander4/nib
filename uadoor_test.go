package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Both surfaces reach one door — `PLAN-accessibility.md` P07.S06's third acceptance clause.
//
// ADR-009's guard shape: routing, not agreement. The UI (`internal/server`) and the CLI
// (`internal/cli`) must each call `uacheck.CheckForUA`, and nothing outside `internal/uacheck` may
// call `uacheck.Check` directly — a handler that did would compose its own refusal, and two
// readings of law 4 that agree today are what ADR-009 exists to prevent.
func TestTheUIAndTheCLIReachTheSameConformanceDoor(t *testing.T) {
	fset := token.NewFileSet()
	doorCalls := map[string]int{}
	var bypasses []string
	scanned := 0
	err := filepath.WalkDir("internal", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == filepath.Join("internal", "uacheck") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		scanned++
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "uacheck" {
				return true
			}
			switch sel.Sel.Name {
			case "CheckForUA":
				doorCalls[strings.Split(filepath.ToSlash(path), "/")[1]]++
			case "Check":
				bypasses = append(bypasses, path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 100 {
		t.Fatalf("the scan read %d non-test file(s) under internal/ — too few to be the tree", scanned)
	}
	for _, surface := range []string{"server", "cli"} {
		if doorCalls[surface] == 0 {
			t.Errorf("internal/%s does not call uacheck.CheckForUA, so that surface does not reach the "+
				"door the other one does — the UI and the CLI must not each decide what conformance means", surface)
		}
	}
	for _, p := range bypasses {
		t.Errorf("%s calls uacheck.Check directly. Call uacheck.CheckForUA, so the refusal is the door's "+
			"and not a second reading composed here (ADR-009)", p)
	}
}
