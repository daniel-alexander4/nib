package nib

import (
	"go/ast"
	"testing"
)

// Every edit of a picture a page draws reaches one door — PLAN-image-edit P01.S02, ADR-219. `internal/objedit`
// refuses a signed document and one carrying a ceremony record, which `pdfops` cannot (it imports neither `sign` nor
// `ceremony`), so a caller of `pdfops.EditPageObjects` anywhere else would skip both.
//
// The routing half (here): `internal/server` and `internal/cli` each call `objedit.Remove` and `objedit.List`, and
// nothing outside `internal/objedit` calls `pdfops.EditPageObjects`. The bypass half — no function inside `pdfops`
// cuts a picture out of a page but the door — is `internal/pdfops`' own
// `TestEveryPageObjectEditRoutesThroughTheDoorInsidePdfops`, which needs the package's types.
func TestEveryPageObjectEditRoutesThroughTheDoor(t *testing.T) {
	doorCalls := map[string]map[string]int{"Remove": {}, "List": {}}
	var bypasses []string
	scanned := walkCallers(t, "internal/objedit", func(file, surface string, f *ast.File) {
		objedit, _ := importNames(f, "nib/internal/objedit")
		pdfops, dot := importNames(f, "nib/internal/pdfops")
		if dot {
			bypasses = append(bypasses, file+" dot-imports pdfops, which hides an EditPageObjects call from this guard")
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch {
			case objedit[pkg.Name] && doorCalls[sel.Sel.Name] != nil:
				doorCalls[sel.Sel.Name][surface]++
			case pdfops[pkg.Name] && sel.Sel.Name == "EditPageObjects":
				bypasses = append(bypasses, file+" (surface "+surface+") calls pdfops.EditPageObjects")
			}
			return true
		})
	})
	if scanned < 100 {
		t.Fatalf("the scan read %d non-test file(s) — too few to be the tree", scanned)
	}
	surfaces := map[string][]string{"Remove": {"server", "cli"}, "List": {"server", "cli"}}
	for door, by := range doorCalls {
		for _, surface := range surfaces[door] {
			if by[surface] == 0 {
				t.Errorf("internal/%s does not call objedit.%s, so a picture edit there does not reach the door that "+
					"refuses a signed or ceremony document", surface, door)
			}
		}
	}
	for _, b := range bypasses {
		t.Errorf("%s directly: route the edit through objedit.Remove, so a signed or ceremony document is refused "+
			"(ADR-219, ADR-009)", b)
	}
}
