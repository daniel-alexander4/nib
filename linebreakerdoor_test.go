package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// breakerExempt is every function outside `mdpdf` that breaks text into lines itself, by "file:function", and
// why it does not go through `mdpdf.BreakGreedy`. Each also says so at the site, on a `//breaker:exempt` line.
var breakerExempt = map[string]string{
	"internal/p2p/readme.go:wrapText":           "emits a token longer than the column alone, where the shared breaker splits or refuses it; the page it sets is signed over",
	"internal/p2p/attestation.go:wrapBlockLine": "hard-breaks a word wider than the line and keeps a half-line floor on word boundaries, which the shared breaker has not; the block it sets is signed over",
}

var wrapperName = regexp.MustCompile(`(?i)^(wrap|break|fold)[A-Za-z]*(text|line|lines|words|para|paragraph|greedy)[A-Za-z]*$|^(wrap|rewrap)$`)

// TestEveryLineBreakerIsTheOneOrNamedExempt — `/pending 796` I3, `PLAN-text-reflow.md` law 4: there is ONE line
// breaker, `mdpdf.BreakGreedy`, which mdpdf's layout and `pdfops`' reflow both call. Two greedy wrappers in
// `internal/p2p` predate it and stay: swapping either changes what a readme page or an attestation block
// looks like, and both are pages parties sign over. They are named here and at their sites; a THIRD is refused.
//
// A function is read as a line wrapper when it is declared outside `mdpdf`, in a non-test file, returns a slice
// of strings (or of slices), and either is NAMED as one (wrap…/break…/fold… + text, line, words…) or measures
// text inside a loop (a call to something named `Width` or `CoreWidth`).
//
// **Declared blind spot:** a wrapper with another name that measures only through a helper of its own is not
// seen. The census is for the next `wrapText`, written the way these two were.
func TestEveryLineBreakerIsTheOneOrNamedExempt(t *testing.T) {
	found := map[string]bool{}
	fset := token.NewFileSet()
	for _, root := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if perr != nil {
				t.Fatalf("%s: %v", path, perr)
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || !returnsLines(fn) {
					continue
				}
				if !wrapperName.MatchString(fn.Name.Name) && !measuresInALoop(fn) {
					continue
				}
				key := filepath.ToSlash(path) + ":" + fn.Name.Name
				found[key] = true
				if _, exempt := breakerExempt[key]; !exempt {
					t.Errorf("%s breaks text into lines itself. There is one line breaker, mdpdf.BreakGreedy "+
						"(mdpdf.Wrap for a plain string): call it, or — if this must differ — name the function in "+
						"breakerExempt with the reason and say so at the site on a //breaker:exempt line", key)
					continue
				}
				if fn.Doc == nil || !strings.Contains(fn.Doc.Text()+docLines(fn.Doc), "breaker:exempt") {
					t.Errorf("%s is exempt from the one line breaker and does not say so where it is declared — "+
						"add a //breaker:exempt line to its comment giving the reason", key)
				}
			}
			return nil
		})
	}
	var stale []string
	for key := range breakerExempt {
		if !found[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("breakerExempt names %s, which is no longer a line wrapper this census finds — take the entry out", key)
	}
}

// docLines is a comment group as written, directive lines included (`CommentGroup.Text` drops `//word:` lines).
func docLines(g *ast.CommentGroup) string {
	var b strings.Builder
	for _, c := range g.List {
		b.WriteString(c.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

// returnsLines reports whether fn's one result is a slice of strings or a slice of slices.
func returnsLines(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return false
	}
	arr, ok := fn.Type.Results.List[0].Type.(*ast.ArrayType)
	if !ok || arr.Len != nil {
		return false
	}
	if id, ok := arr.Elt.(*ast.Ident); ok {
		return id.Name == "string"
	}
	_, nested := arr.Elt.(*ast.ArrayType)
	return nested
}

// measuresInALoop reports whether fn calls something named Width or CoreWidth inside a for or range loop.
func measuresInALoop(fn *ast.FuncDecl) bool {
	hit := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		var body *ast.BlockStmt
		switch l := n.(type) {
		case *ast.ForStmt:
			body = l.Body
		case *ast.RangeStmt:
			body = l.Body
		default:
			return true
		}
		ast.Inspect(body, func(m ast.Node) bool {
			call, ok := m.(*ast.CallExpr)
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
			if name == "Width" || name == "CoreWidth" {
				hit = true
			}
			return true
		})
		return true
	})
	return hit
}
