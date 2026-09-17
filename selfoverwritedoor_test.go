package nib

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryFolderFillingWriterAsksWhetherItIsAboutToWriteItsOwnSource — /pending 569, ADR-009.
//
// # The rule
//
// A writer that fills a folder the user named, with file names IT derived, must not write over the
// document those files were derived from. Two writers had made that decision for themselves —
// which is to say neither had made it — and both were measured destroying a user's document:
// `nib split foo1-2.pdf --out-dir . --ranges 1-2 --prefix foo` replaced the 105,102-byte input
// with the 94,254-byte part at exit 0, and the GUI's split replaced the open document's own file
// at status 200. `nib fill --out-dir` was a third caller of the first writer and did the same to
// the blank form. ADR-009: the rule is written ONCE (`pdfops.OutputOverwritingSource`) and every
// site calls it.
//
// # What it asserts, and why it is not a count of call sites
//
// ADR-009 again: *"the guard asserts routing through the door, not the text each site prints —
// eight copies checked for agreement say nothing about a ninth site added without one."* A floor
// on `OutputOverwritingSource` calls would be satisfied by the two sites that already exist while
// a third arrived beside them. So this DISCOVERS the population and requires every member to
// either route through the door or carry a named exemption.
//
// # The population, and why THIS predicate finds it
//
// `os.MkdirAll(<a bare identifier>, …)` — a whole directory created from a variable that is the
// destination itself, rather than from `filepath.Dir(path)`. That is exactly "a folder the user
// named, about to be filled", and it separates the five writers that fill one from the four that
// merely ensure a parent directory exists for a single known file (`delivery.go`, `session.go`).
// The alternative — listing the writers — is the shape `atomicdurable_test.go` refuses one level
// down: *"a door added to `atomicfile` and not to a hand-written list is a door this guard does
// not police."*
//
// # What this cannot see, so it is said rather than implied
//
// It cannot tell a CORRECT call to the door from a call whose result is ignored, and it cannot
// judge an exemption's reasoning — a site can silence it with a comment. What it makes impossible
// is the silent case: a sixth folder-filling writer arriving with neither, which is how both of
// the measured defects got in. The behaviour is owned by
// `internal/cli/splitcollide_test.go` and `internal/server/splitcollide_test.go`, which drive the
// real doors and were red against the tree before this.
func TestEveryFolderFillingWriterAsksWhetherItIsAboutToWriteItsOwnSource(t *testing.T) {
	const door = "OutputOverwritingSource"
	const exempt = "SELF-OVERWRITE EXEMPT:"

	type site struct{ pkg, fn, where string }
	var missing []string
	population, routed, exempted := 0, 0, 0
	var scanned int

	for _, dir := range []string{"internal/cli", "internal/server"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			n := e.Name()
			if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
				continue
			}
			path := filepath.Join(dir, n)
			fset := token.NewFileSet()
			// ParseComments, because an exemption is a comment and mode 0 discards them.
			f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if perr != nil {
				t.Fatalf("parse %s: %v", path, perr)
			}
			scanned++

			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				fills, calls := false, false
				ast.Inspect(fd.Body, func(nd ast.Node) bool {
					call, ok := nd.(*ast.CallExpr)
					if !ok {
						return true
					}
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						if sel.Sel.Name == door {
							calls = true
						}
						// os.MkdirAll(dir, …) where dir is a plain identifier: the
						// destination itself, not filepath.Dir(someFile).
						if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && sel.Sel.Name == "MkdirAll" {
							if _, bare := call.Args[0].(*ast.Ident); bare {
								fills = true
							}
						}
					}
					return true
				})
				if !fills {
					continue
				}
				population++
				s := site{dir, fd.Name.Name, fmt.Sprintf("%s:%d", path, fset.Position(fd.Pos()).Line)}
				if calls {
					routed++
					continue
				}
				// Anywhere in the declaration — the doc comment counts too.
				//
				// **This used to require the marker INSIDE the body**, on the argument that a
				// doc comment survives a rewrite of everything below it. A mutation probe
				// widened the range and the guard did not notice, because no site puts its
				// marker in a doc comment: the two spellings select the same set today, so the
				// stricter rule was policing nothing while reading as though it policed
				// something. A distinction nothing tests is one the sixth site will get wrong,
				// and the wide range is what a reader would guess.
				//
				// `fd.Doc` is checked explicitly rather than by widening the position range:
				// `FuncDecl.Pos()` is the `func` keyword, so a doc comment sits BEFORE it and a
				// range test alone silently keeps the old behaviour under a comment claiming
				// otherwise — which is how the first attempt at this fix was written.
				marked := fd.Doc != nil && strings.Contains(fd.Doc.Text(), exempt)
				for _, cg := range f.Comments {
					if marked {
						break
					}
					if cg.Pos() > fd.Body.Pos() && cg.End() < fd.Body.End() &&
						strings.Contains(cg.Text(), exempt) {
						marked = true
					}
				}
				if marked {
					exempted++
					continue
				}
				missing = append(missing, fmt.Sprintf("%s (%s)", s.fn, s.where))
			}
		}
	}

	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("%s creates a destination folder and fills it with derived names, and neither "+
			"calls pdfops.%s nor carries a %q comment.\n\t"+
			"A name nib derived can be the document it was derived from: measured, `nib split` "+
			"replaced its 105,102-byte input with a 94,254-byte part at exit 0. Check every output "+
			"path against the source BEFORE the first write, or name the exemption here — a "+
			"superset output (pagenum) and a typed-then-confirmed name (save as) are both exempt, "+
			"and both say so at the site.", m, door, exempt)
	}

	// ── The stimulus ────────────────────────────────────────────────────────────────────────────
	//
	// An empty population reports perfect compliance forever, and this guard has two ways to
	// reach one: the AST predicate stops matching, or the directories stop being read. Both are
	// checked, and so is each ARM — a run where nothing routes through the door is the regression
	// this exists for, and it looks identical to a clean run from `missing` alone.
	if scanned < 20 {
		t.Fatalf("the scan read %d source file(s) across internal/cli and internal/server — it is "+
			"not reading the tree, and every verdict above is about nothing", scanned)
	}
	if population < 5 {
		t.Errorf("found %d folder-filling writer(s); the census when this guard was written was 5 "+
			"(the two splits, pagenum --continuous, save as, the release download). Fewer means "+
			"the `os.MkdirAll(<ident>, …)` predicate has stopped matching what it is about",
			population)
	}
	if routed < 2 {
		t.Errorf("%d writer(s) route through pdfops.%s; the two splits both must. A writer that "+
			"stops calling the door reads here as one that never filled a folder",
			routed, door)
	}
	if exempted < 3 {
		t.Errorf("%d writer(s) carry a named exemption, want at least 3 — an exemption that "+
			"disappears means either the site started routing (raise this) or the comment was "+
			"reworded, which turns the marker into a check nobody is running", exempted)
	}
}
