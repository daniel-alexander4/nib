package pdfread_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryMergeLoopUnionsOptionalContent — ADR-009, ADR-131 (`/pending 870`): a merge keeps the first document's
// catalog, so a later document's `/OCProperties` — the one thing that says its hidden layers are hidden — is gone
// unless the loop joins it to the host's, and a layer nobody lists is DRAWN. Two loops merge today and both call
// `UnionOptionalContent`; this is what makes a third one a decision. The sibling of
// `TestEveryWriteRoutesThroughTheDoor`, which asks the same loops for `PutBackValidatorLosses`.
//
// It checks ROUTING, over every non-test Go file in the module: a function that names `pdfcpu.MergeXRefTables`
// also names `UnionOptionalContent`, or says why not in a comment of its own holding `optionalmerge:exempt`.
func TestEveryMergeLoopUnionsOptionalContent(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	loops, exempt := 0, 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			switch d.Name() {
			case "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			t.Fatal(perr)
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			merges, unions := false, false
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.SelectorExpr:
					merges = merges || v.Sel.Name == "MergeXRefTables"
					unions = unions || v.Sel.Name == "UnionOptionalContent"
				case *ast.Ident: // inside pdfread the door is called by its bare name
					unions = unions || v.Name == "UnionOptionalContent"
				}
				return true
			})
			if !merges {
				continue
			}
			loops++
			if unions {
				continue
			}
			named := false
			for _, cg := range file.Comments {
				if cg.Pos() >= fd.Pos() && cg.End() <= fd.End() && strings.Contains(cg.Text(), "optionalmerge:exempt") {
					named = true
				}
			}
			if named {
				exempt++
				continue
			}
			t.Errorf("%s: %s merges documents (pdfcpu.MergeXRefTables) and never calls pdfread.UnionOptionalContent — "+
				"a later document's hidden layers are then drawn in the merged one (ADR-131). Call it after the "+
				"merge with the source's dictionary held from before it (SourceOptionalContent), or name the "+
				"exemption inside the function: // optionalmerge:exempt <why>", filepath.ToSlash(rel), fd.Name.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the walk met the loops it is here to judge.
	if loops < 2 {
		t.Fatalf("found %d functions that merge documents, want the two there are — the guard is not reading the module", loops)
	}
	if exempt != 0 {
		t.Logf("%d of %d merge loops are exempt by name", exempt, loops)
	}
}
