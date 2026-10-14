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
			case "node_modules", "vendor", "testdata", "third_party":
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

// TestEveryMergeLoopRefusesASourceItCannotCarryWhole — ADR-009, ADR-164, ADR-145 (`/pending 883`): what `Write`
// knows of a context — a name tree nib could not read whole, pages that are a placeholder for a dynamic XFA form —
// is kept against that context and does not follow its objects into the host a merge moves them to. So each loop
// asks of every SOURCE, before the merge, what `Write` would have asked: `RefuseNameTreeLoss` and
// `RefuseDynamicXFA`. Both loops do; this is what makes a third one a decision, as its sibling above does for
// optional content. Routing only, by the same walk; an exemption is named `mergesource:exempt` in the function.
func TestEveryMergeLoopRefusesASourceItCannotCarryWhole(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	doors := []string{"RefuseNameTreeLoss", "RefuseDynamicXFA"}
	loops := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			switch d.Name() {
			case "node_modules", "vendor", "testdata", "third_party":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if perr != nil {
			t.Fatal(perr)
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			merges, named := false, map[string]bool{}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.SelectorExpr:
					merges = merges || v.Sel.Name == "MergeXRefTables"
					named[v.Sel.Name] = true
				case *ast.Ident: // inside pdfread a door is called by its bare name
					named[v.Name] = true
				}
				return true
			})
			if !merges {
				continue
			}
			loops++
			exempt := false
			for _, cg := range file.Comments {
				if cg.Pos() >= fd.Pos() && cg.End() <= fd.End() && strings.Contains(cg.Text(), "mergesource:exempt") {
					exempt = true
				}
			}
			for _, door := range doors {
				if !named[door] && !exempt {
					t.Errorf("%s: %s merges documents (pdfcpu.MergeXRefTables) and never calls pdfread.%s of the "+
						"source — what Write would refuse of that document is not known of the host it is merged "+
						"into, so the merged file is written without it and nothing says so. Ask it before the "+
						"merge, or name the exemption inside the function: // mergesource:exempt <why>",
						filepath.ToSlash(rel), fd.Name.Name, door)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if loops < 2 {
		t.Fatalf("found %d functions that merge documents, want the two there are — the guard is not reading the module", loops)
	}
}
