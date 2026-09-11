package nib

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The shared scan behind the authoring-door guards — `titledoor_test.go` (P03.S01) and
// `langdoor_test.go` (P03.S02).
//
// **It is shared for the reason ADR-009 gives about the code it polices.** Two guards over one
// population, each with its own copy of "what counts as an authoring call site", is two definitions
// that drift — and the drift would be invisible, because each guard would keep passing against its
// own idea of the population. One scan, two rules.

// authoringPrimitives are the three functions that make a PDF out of something that was not one.
// Every document nib authors comes out of one of these.
var authoringPrimitives = map[string]bool{
	"CreateFromJSON":  true,
	"ImagesToPDF":     true,
	"ConvertDocToPDF": true,
}

type authoringSite struct {
	key  string // "<repo-relative file>:<enclosing function>" — how both guards' tables are keyed
	pos  string // "<file>:<line>" of the call itself
	ctor string // which primitive
	// calls is every function name called anywhere in the enclosing function, so a guard can ask
	// "does this site also reach <door>?" without re-walking the tree.
	calls map[string]bool
}

// scanAuthoringSites walks every non-test Go file under the repo and returns one entry per call of
// an authoring primitive, attributed to the function that encloses it.
//
// Attribution is by construction: the walk starts at one `FuncDecl`'s body, so a call inside a
// nested closure still belongs to the function enclosing it — which is also where a door call
// would live. Parsing with mode 0 attaches no comments and `ast.Inspect` never enters a string
// literal, so neither a comment nor a string can launder a site in or out (the reasoning
// `zerocaller_test.go` records at length).
func scanAuthoringSites(t *testing.T) (sites []authoringSite, declaredIn map[string]string, calledOutside map[string]int) {
	t.Helper()
	repo, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	declaredIn = map[string]string{}
	calledOutside = map[string]int{}
	files := 0

	walkErr := filepath.WalkDir(repo, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "web", "test", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, p, nil, 0)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", p, perr)
		}
		files++
		rel, _ := filepath.Rel(repo, p)
		inPdfops := strings.HasPrefix(rel, filepath.Join("internal", "pdfops")+string(filepath.Separator))

		for _, dd := range f.Decls {
			fd, ok := dd.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			if authoringPrimitives[fd.Name.Name] && inPdfops {
				declaredIn[fd.Name.Name] = fmt.Sprintf("%s:%d", rel, fset.Position(fd.Name.Pos()).Line)
			}
			calls := map[string]bool{}
			var found []authoringSite
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := calleeName(call.Fun)
				if name == "" {
					return true
				}
				calls[name] = true
				if !authoringPrimitives[name] {
					return true
				}
				if !inPdfops {
					calledOutside[name]++
				}
				found = append(found, authoringSite{
					key:  rel + ":" + fd.Name.Name,
					pos:  fmt.Sprintf("%s:%d", rel, fset.Position(call.Pos()).Line),
					ctor: name,
				})
				return true
			})
			// calls is complete only after the whole body is walked, so it is attached last.
			for i := range found {
				found[i].calls = calls
			}
			sites = append(sites, found...)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	if files < 50 {
		t.Fatalf("the scan read %d files — it is not reading the tree", files)
	}
	return sites, declaredIn, calledOutside
}

// assertScanIsNotVacuous is the anti-vacuity floor both guards share: a routing scan that matches
// nothing passes silently, and the two ways that happens are a renamed primitive and a primitive
// nothing calls. A site COUNT is deliberately not asserted — removing a caller is legitimate.
func assertScanIsNotVacuous(t *testing.T, declaredIn map[string]string, calledOutside map[string]int) {
	t.Helper()
	for name := range authoringPrimitives {
		if declaredIn[name] == "" {
			t.Errorf("no FuncDecl for authoring primitive %q under internal/pdfops — renamed or "+
				"removed, and this guard has been matching nothing", name)
		}
		if calledOutside[name] == 0 {
			t.Errorf("authoring primitive %q has no call site outside internal/pdfops — either it "+
				"is dead or the scan is blind to how it is called", name)
		}
	}
}

// calleeName is the function name of a call, whether it is called bare (`ImagesToPDF(...)`) or
// through its package (`pdfops.ImagesToPDF(...)`). Method calls on a value resolve the same way and
// are harmless here: no type under scan has a method named after a primitive or a door.
func calleeName(fun ast.Expr) string {
	switch e := fun.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}
