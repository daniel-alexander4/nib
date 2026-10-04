package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestTheNUpCarriesReadTheSourceAsNUpDoes — `/pending 730`: both n-up captures match content byte for byte
// against forms `api.NUp` built from ITS read (`ReadAndValidate`), so both read through `nupSourceRead`. The tag
// carry read through the optimizing `inspectionRead` and agreed with the note carry only because pdfcpu's optimize
// pass does not touch content streams today; a census over the two functions holds them on the one door.
func TestTheNUpCarriesReadTheSourceAsNUpDoes(t *testing.T) {
	want := map[string]string{"capturePageSources": "tagcarry.go", "captureNoteAnnots": "annotcarry.go"}
	fset := token.NewFileSet()
	seen := 0
	for fn, file := range want {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			decl, ok := d.(*ast.FuncDecl)
			if !ok || decl.Name.Name != fn || decl.Body == nil {
				continue
			}
			seen++
			var door bool
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch x := call.Fun.(type) {
				case *ast.Ident:
					if x.Name == "nupSourceRead" {
						door = true
					}
					if x.Name == "inspectionRead" {
						t.Errorf("%s reads its source through inspectionRead, the optimizing read api.NUp does not perform", fn)
					}
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && id.Name == "pdfread" && x.Sel.Name != "Pages" {
						t.Errorf("%s calls pdfread.%s itself; read the n-up source through nupSourceRead", fn, x.Sel.Name)
					}
				}
				return true
			})
			if !door {
				t.Errorf("%s does not read its source through nupSourceRead", fn)
			}
		}
	}
	if seen != len(want) {
		t.Fatalf("census found %d of the %d n-up captures; it is not reading them", seen, len(want))
	}
}
