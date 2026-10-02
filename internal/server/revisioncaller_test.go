package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheSignedVersionIsWalkedOnlyOnDemand — D10: recovery runs when the user asks, never on document open, because
// a walk is up to one `Revisions` per candidate prefix over the whole file. The route's handler is the one production
// site that names `sign.SignedRevisionFor`; a reference anywhere else (an open path, `docResponse`) is a second door.
// The jsdom half holds the client to the same (signedrevision.test.mjs).
func TestTheSignedVersionIsWalkedOnlyOnDemand(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	var sites []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == "third_party" || strings.HasPrefix(n, ".") && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, p, nil, 0)
		if perr != nil {
			return perr
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "SignedRevisionFor" {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "sign" {
						sites = append(sites, fn.Name.Name+" "+fset.Position(sel.Pos()).String())
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || !strings.HasPrefix(sites[0], "handleDocumentRevision ") {
		t.Fatalf("sign.SignedRevisionFor is named at %v; want exactly one site, in handleDocumentRevision", sites)
	}
}
