package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestEveryPageAnnotationWalkGoesThroughTheOneDoor — `eachPageAnnot` is the one walk over pages' annotations
// (ADR-009): it visits a shared page, `/Annots` array or annotation once. A function that walks pages with
// `eachPage` and reads `/Annots` itself rebuilds the pages × slots cost the phase-close review of
// PLAN-returned-document P02 measured (15 KB held Scan for 3 min 46 s). The page-set operations walk pdfcpu's
// own page list, not `eachPage`, and are not this door's.
func TestEveryPageAnnotationWalkGoesThroughTheOneDoor(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var door, checked int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var walksPages, readsAnnots bool
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "eachPage" {
						walksPages = true
					}
				case *ast.BasicLit:
					if x.Kind == token.STRING && x.Value == `"Annots"` {
						readsAnnots = true
					}
				}
				return true
			})
			if !walksPages {
				continue
			}
			checked++
			if fn.Name.Name == "eachPageAnnot" {
				door++
				continue
			}
			if readsAnnots {
				t.Errorf("%s (%s) walks pages with eachPage and reads /Annots itself; route it through eachPageAnnot",
					fn.Name.Name, fset.Position(fn.Pos()))
			}
		}
	}
	// The stimulus: the door exists and the census saw page walks besides it.
	if door != 1 || checked < 3 {
		t.Fatalf("census saw %d door and %d page walks; it is not reading the package", door, checked)
	}
}
