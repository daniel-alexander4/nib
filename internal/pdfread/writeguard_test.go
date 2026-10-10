package pdfread_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEveryWriteRoutesThroughTheDoor — ADR-009, ADR-129: a font pdfcpu's validator took out of a document is put
// back by `pdfread.Write` and by nothing else, so a context serialised any other way is written without it
// (`/pending 842`). It checks ROUTING, over every non-test Go file in the module:
//
//  1. No file names pdfcpu's writers (`api.WriteContext`, `api.Write`, `api.WriteIncr`, `api.WriteIncrement`,
//     `pdfcpu.WriteContext`, `pdfcpu.WriteIncrement`) except this package's `write.go`.
//  2. A function that names a pdfcpu call which moves a context's objects somewhere a remembered reference no
//     longer names them — the merge renumbers them, a cut, an extraction, n-up and booklet copy them — also
//     names `PutBackValidatorLosses`, which must run first.
func TestEveryWriteRoutesThroughTheDoor(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	writersAPI := map[string]bool{"WriteContext": true, "Write": true, "WriteIncr": true, "WriteIncrement": true}
	writersPdfcpu := map[string]bool{"WriteContext": true, "WriteIncrement": true}
	movers := map[string]bool{"MergeXRefTables": true, "CutPage": true, "NUpFromPDF": true, "ExtractPages": true, "BookletFromPDF": true}
	doorCalls, moverSites, theDoor := 0, 0, 0
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
		rel = filepath.ToSlash(rel)
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		inDoor := strings.HasPrefix(rel, "internal/pdfread/")
		names := map[string]string{} // local import name -> which pdfcpu package
		for _, im := range file.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			local := p[strings.LastIndex(p, "/")+1:]
			if im.Name != nil {
				local = im.Name.Name
			}
			switch p {
			case "github.com/pdfcpu/pdfcpu/pkg/api":
				names[local] = "api"
			case "github.com/pdfcpu/pdfcpu/pkg/pdfcpu":
				names[local] = "pdfcpu"
			case "nib/internal/pdfread":
				names[local] = "pdfread"
			}
		}
		selOf := func(n ast.Node) (pkg, name string, pos token.Pos) {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return "", "", 0
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return "", "", 0
			}
			return names[x.Name], sel.Sel.Name, sel.Pos()
		}
		ast.Inspect(file, func(n ast.Node) bool {
			pkg, name, pos := selOf(n)
			switch {
			case pkg == "api" && writersAPI[name], pkg == "pdfcpu" && writersPdfcpu[name]:
				if rel == "internal/pdfread/write.go" {
					theDoor++
					return true
				}
				t.Errorf("%s:%d: names %s.%s — write through pdfread.Write, which first puts back the fonts "+
					"pdfcpu's validator took out (ADR-129)", rel, fset.Position(pos).Line, pkg, name)
			case pkg == "pdfread" && name == "Write":
				doorCalls++
			}
			return true
		})
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			moves, restores := "", false
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if pkg, name, _ := selOf(n); pkg == "pdfcpu" && movers[name] {
					moves = name
				} else if pkg == "pdfread" && name == "PutBackValidatorLosses" {
					restores = true
				}
				if id, ok := n.(*ast.Ident); ok && inDoor && id.Name == "PutBackValidatorLosses" {
					restores = true
				}
				return true
			})
			if moves == "" {
				continue
			}
			moverSites++
			if !restores {
				t.Errorf("%s: %s calls pdfcpu.%s, which moves the context's objects, without "+
					"pdfread.PutBackValidatorLosses before it — a font the validator took out is then lost for good "+
					"(ADR-129)", rel, fd.Name.Name, moves)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the walk met the door, its callers and the movers it is here to judge.
	if theDoor != 1 || doorCalls < 10 || moverSites < 4 {
		t.Fatalf("found %d writer in write.go, %d calls of pdfread.Write and %d functions that move a context's "+
			"objects — the guard is not reading the module", theDoor, doorCalls, moverSites)
	}
}
