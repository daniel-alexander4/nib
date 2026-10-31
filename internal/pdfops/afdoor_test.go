package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestForgetAssociatedFilesHasOneNamedCaller — D6 pin 1 (the census in the style of `TestEveryWriteRoutesThroughTheDoor`).
// Taking the associated-file mark off a context says "the user asked for these files to go": `removeAllAttachments`
// is the one place that asks it, having stripped every `/AF` key first (`dropAssociatedFileKeys`), and the strip's
// only caller is the same function. A second caller would write a document without the refusal and without the strip.
func TestForgetAssociatedFilesHasOneNamedCaller(t *testing.T) {
	calls := map[string]map[string]int{"ForgetAssociatedFiles": {}, "dropAssociatedFileKeys": {}}
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", ".claude", "testdata", "third_party":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if perr != nil {
			return perr
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch f := call.Fun.(type) {
				case *ast.SelectorExpr:
					name = f.Sel.Name
				case *ast.Ident:
					name = f.Name
				}
				if m, watched := calls[name]; watched {
					m[fn.Name.Name]++
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for door, m := range calls {
		if len(m) != 1 || m["removeAllAttachments"] != 1 {
			t.Errorf("%s is called from %v, want exactly removeAllAttachments once — a second caller writes a document "+
				"that lost its associated files without the strip or the refusal (D6, ADR-136)", door, m)
		}
	}
}
