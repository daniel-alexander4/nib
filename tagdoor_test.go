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

// Every structure write reaches one door — `PLAN-accessibility.md` P10.S02.
//
// ADR-009's guard shape: routing. `internal/server` and `internal/cli` must each call `tagwrite.Commit`
// and `tagwrite.Edit`, and nothing outside `internal/tagwrite` may call `pdfops.CommitTags` or
// `pdfops.EditStructure` — a caller that did would skip the signed refusal, or compose its own.
func TestEveryStructureWriteReachesTheSignedDocumentDoor(t *testing.T) {
	fset := token.NewFileSet()
	doorCalls := map[string]map[string]int{"Commit": {}, "Edit": {}}
	var bypasses []string
	scanned := 0
	err := filepath.WalkDir("internal", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == filepath.Join("internal", "tagwrite") {
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
		surface := strings.Split(filepath.ToSlash(path), "/")[1]
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
			case pkg.Name == "tagwrite" && doorCalls[sel.Sel.Name] != nil:
				doorCalls[sel.Sel.Name][surface]++
			case pkg.Name == "pdfops" && (sel.Sel.Name == "CommitTags" || sel.Sel.Name == "EditStructure"):
				bypasses = append(bypasses, path+" calls pdfops."+sel.Sel.Name)
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
	for door, by := range doorCalls {
		for _, surface := range []string{"server", "cli"} {
			if by[surface] == 0 {
				t.Errorf("internal/%s does not call tagwrite.%s, so a structure write there does not reach the "+
					"door that refuses a signed document", surface, door)
			}
		}
	}
	for _, b := range bypasses {
		t.Errorf("%s directly. Call the tagwrite door, so a signed document is refused there and not "+
			"re-checked here (ADR-009)", b)
	}
}
