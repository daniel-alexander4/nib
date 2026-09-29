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

// TestEveryPageContentReadRoutesThroughTheDoor — ADR-056 under ADR-009: a divided `/Contents` is read correctly
// only if nothing reads a page's content around `pdfread.PageContent`. An AST census over every non-test Go file
// in the module outside this package: a call to any method named `PageContent` — pdfcpu's, on a `*model.Context`
// or an `*XRefTable` — is a finding unless its line carries `//pagecontent:exempt <name>` AND the name is one of
// the exemptions listed here, each at the file it is declared for. Calls to the door itself (`pdfread.PageContent`,
// through the import's local name) are counted, so a census that stopped seeing them reads red, not green.
func TestEveryPageContentReadRoutesThroughTheDoor(t *testing.T) {
	exempt := map[string]string{
		"ContentDigest": "internal/pdfops/attachments.go", // ADR-013: the digest's coverage is a format
		"uacheck":       "internal/uacheck/content.go",    // ADR-052: veraPDF's join is unmeasured (/pending 719)
		// Both compare with the form `api.NUp` wrote from pdfcpu's own join, so they must read that join.
		"annotcarry-nup": "internal/pdfops/annotcarry.go",
		"tagcarry-nup":   "internal/pdfops/tagcarry.go",
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	routed, seenExempt := 0, map[string]int{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			switch d.Name() {
			case "node_modules", "vendor", "testdata", "pdfread":
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
		file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			t.Fatal(perr)
		}
		door := ""
		for _, im := range file.Imports {
			if p, _ := strconv.Unquote(im.Path.Value); p == "nib/internal/pdfread" {
				door = "pdfread"
				if im.Name != nil {
					door = im.Name.Name
				}
			}
		}
		marks := map[int]string{}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if rest, ok := strings.CutPrefix(c.Text, "//pagecontent:exempt"); ok {
					name, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
					marks[fset.Position(c.Pos()).Line] = name
				}
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "PageContent" {
				return true
			}
			if id, isID := sel.X.(*ast.Ident); isID && door != "" && id.Name == door {
				routed++
				return true
			}
			line := fset.Position(call.Pos()).Line
			name, marked := marks[line]
			switch {
			case !marked:
				t.Errorf("%s:%d reads a page's content through pdfcpu's PageContent, which joins a /Contents array "+
					"with no separator — read it through pdfread.PageContent (ADR-056)", rel, line)
			case exempt[name] != rel:
				t.Errorf("%s:%d is marked //pagecontent:exempt %q, which is not an exemption declared for this file "+
					"in TestEveryPageContentReadRoutesThroughTheDoor", rel, line, name)
			default:
				seenExempt[name]++
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The stimulus: the census must be SEEING the door's callers, or a walk that parsed nothing passes.
	if routed < 13 {
		t.Errorf("the census saw %d calls to pdfread.PageContent; at ADR-056 there were 16 — it is not reading the tree", routed)
	}
	// Each exemption excuses exactly ONE call: a name that matched none is stale and excuses whatever lands there
	// next, and one that matched two is excusing a call nobody declared.
	for name, file := range exempt {
		if n := seenExempt[name]; n != 1 {
			t.Errorf("the exemption %q (%s) matched %d calls, want exactly 1", name, file, n)
		}
	}
}
