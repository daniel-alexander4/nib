package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoPackageLevelPdfcpuConfiguration — ADR-183. `cli.Run` points pdfcpu at a private configuration root before any
// verb runs, and pdfcpu caches the FIRST configuration it builds. A package-level variable or an init function that
// builds one would run before `Run` does, create `$XDG_CONFIG_HOME/pdfcpu` itself and leave every later call answering
// from it. This reads every non-test file of the module for exactly that.
func TestNoPackageLevelPdfcpuConfiguration(t *testing.T) {
	builds := func(n ast.Node) (pos token.Pos, found bool) {
		ast.Inspect(n, func(x ast.Node) bool {
			if c, ok := x.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "NewDefaultConfiguration" || sel.Sel.Name == "NewAESConfiguration" || sel.Sel.Name == "NewRC4Configuration") && !found {
					pos, found = c.Pos(), true
				}
			}
			return !found
		})
		return
	}
	fset := token.NewFileSet()
	files, seen := 0, 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "third_party", "testdata", ".git", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		files++
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				seen++
				if pos, found := builds(d); found {
					t.Errorf("%s: a package-level variable builds a pdfcpu configuration before cli.Run can point it at its private root", fset.Position(pos))
				}
			case *ast.FuncDecl:
				if d.Name.Name == "init" && d.Recv == nil && d.Body != nil {
					seen++
					if pos, found := builds(d.Body); found {
						t.Errorf("%s: init builds a pdfcpu configuration before cli.Run can point it at its private root", fset.Position(pos))
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 || seen < 50 {
		t.Errorf("read %d files and %d package-level declarations: the scan is not looking at the module", files, seen)
	}
}
