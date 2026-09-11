package nib

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryAuthoredDocumentGetsATitle — `PLAN-accessibility.md` P03.S01.T03.
//
// # The rule
//
// Nib authors PDFs from three primitives — `CreateFromJSON`, `ImagesToPDF` and `ConvertDocToPDF`.
// Every document nib authors and then HANDS TO SOMEONE must carry the catalog floor (PDF/UA 7.1 t8,
// t9, t10), and the one door onto that floor is `pdfops.SetTitle`. So: every call site of an
// authoring primitive either routes through the door, or is NAMED here with the reason it is a
// fragment rather than a document.
//
// # Why it is written this way, and not as eight copies of a check
//
// ADR-009: a rule holding at more than one call site is written once and the guard asserts the
// ROUTING, not the text each site prints. Checking that five sites agree says nothing about a sixth
// added without one — which is the failure this exists to make impossible. The exemption map is
// read in both directions so a row that stops matching a real site goes red rather than rotting.
//
// # Anti-vacuity
//
// Two floors, because a scan that matches nothing passes silently. The constructor names must each
// resolve to a real `FuncDecl` in `internal/pdfops` (a rename goes red here instead of quietly
// matching nothing), and each must be found called at least once outside its own package. A bare
// site COUNT is deliberately not asserted: removing a caller is legitimate and should not be a test
// failure, while a constructor that nothing calls is exactly the silence to catch.
//
// # Declared blind spot
//
// Routing is "the enclosing function also calls SetTitle", not dataflow: the guard does not prove
// the title is applied to THAT primitive's output. Proving it needs type-checked dataflow, which is
// a large instrument for a one-line mistake that the compiler and `title_test.go`'s behavioural
// tests both reach first. What this catches is the case those miss — a new authoring site that
// calls the door nowhere at all.
func TestEveryAuthoredDocumentGetsATitle(t *testing.T) {
	// The authoring primitives. Each makes a PDF out of something that was not one.
	constructors := map[string]bool{
		"CreateFromJSON":  true,
		"ImagesToPDF":     true,
		"ConvertDocToPDF": true,
	}

	// The named exemptions, keyed `<file>:<enclosing function>`. Every row carries its reason.
	exempt := map[string]string{
		"internal/pdfops/pdfops.go:RedactPages": "a one-page raster FRAGMENT, re-merged into the " +
			"document being redacted and never handed to anyone on its own. Titling it would put a " +
			"dc:title on an intermediate that is discarded three lines later, and the redacted " +
			"document keeps whatever title it arrived with.",
	}

	repo, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()

	type site struct{ key, pos, ctor string }
	var sites []site
	declared := map[string]string{}   // constructor name -> where it is declared
	calledOutside := map[string]int{} // constructor name -> call sites outside internal/pdfops
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
			if constructors[fd.Name.Name] && inPdfops {
				declared[fd.Name.Name] = fmt.Sprintf("%s:%d", rel, fset.Position(fd.Name.Pos()).Line)
			}
			// Attribution is by construction: the walk starts at one FuncDecl's body, so a call
			// inside a nested closure still belongs to the function that encloses it — which is
			// also where its SetTitle would live.
			callsDoor := false
			var found []site
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := calleeName(call.Fun)
				if name == "SetTitle" {
					callsDoor = true
				}
				if !constructors[name] {
					return true
				}
				// A constructor's own declaration is not a call site; only calls reach here.
				if !inPdfops {
					calledOutside[name]++
				}
				found = append(found, site{
					key:  rel + ":" + fd.Name.Name,
					pos:  fmt.Sprintf("%s:%d", rel, fset.Position(call.Pos()).Line),
					ctor: name,
				})
				return true
			})
			if callsDoor {
				continue
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

	// Floor 1: the primitives are still spelled the way this guard spells them.
	for name := range constructors {
		if declared[name] == "" {
			t.Errorf("no FuncDecl for authoring primitive %q under internal/pdfops — renamed or "+
				"removed, and this guard has been matching nothing", name)
		}
	}
	// Floor 2: each is actually reached from outside its own package.
	for name := range constructors {
		if calledOutside[name] == 0 {
			t.Errorf("authoring primitive %q has no call site outside internal/pdfops — either it "+
				"is dead or the scan is blind to how it is called", name)
		}
	}

	// The routing rule, and the exemption map read both ways.
	hit := map[string]bool{}
	var bad []string
	for _, s := range sites {
		if reason, ok := exempt[s.key]; ok {
			hit[s.key] = true
			_ = reason
			continue
		}
		bad = append(bad, fmt.Sprintf("%s calls %s and never reaches pdfops.SetTitle", s.pos, s.ctor))
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("authored document with no title: %s\n\tEither route it through pdfops.SetTitle "+
			"with a title the caller knows, or add a row to `exempt` in this file naming the "+
			"reason it is a fragment rather than a document.", b)
	}
	for key := range exempt {
		if !hit[key] {
			t.Errorf("stale exemption %q: no un-titled authoring call site is attributed to it. "+
				"The site moved, was renamed, or now routes through the door — remove the row.", key)
		}
	}
}

// calleeName is the function name of a call, whether it is called bare (`ImagesToPDF(...)`) or
// through its package (`pdfops.ImagesToPDF(...)`). Method calls on a value resolve the same way and
// are harmless here: no type under scan has a method named after an authoring primitive.
func calleeName(fun ast.Expr) string {
	switch e := fun.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}
