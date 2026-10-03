package pdfread_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEveryValidatingReadRoutesThroughTheDoor — ADR-009: the reference door (`/pending 675`, `/pending 764`) and the optimize
// budget (`/pending 706`, `/pending 714`) hold only if nothing reaches pdfcpu's validator or its optimize pass
// around this package. It checks ROUTING, over every non-test Go file in the module outside this package:
//
//  1. No call to pdfcpu's validating reads or its pass (`api.ReadAndValidate`, `api.ReadValidateAndOptimize`,
//     `api.ValidateContext`, `api.OptimizeContext`, `pdfcpu.OptimizeXRefTable`), and no import of pdfcpu's
//     `validate` package — read through `Validated`/`ReadOptimized`, optimize through `Optimize`.
//  2. No `api` function that reads a document itself (ADR-082, `/pending 716`): every exported function of pdfcpu's
//     `api` package with an `io.ReadSeeker` or `[]io.ReadSeeker` parameter or an `inFile…` path parameter, READ FROM
//     THE PDFCPU SOURCE THE MODULE BUILDS AGAINST, plus every `api.*File` function. Outside this package such a
//     function may be named only in a call that passes the literal `nil` at every reader position (`api.Create`'s
//     and `api.ImportImages`' base document) — or it is `api.ReadContext`, the unvalidated read, which follows no
//     reference chain. Each one nib needs is restated in `apiread.go` over `Validated`/`ReadOptimized`.
//
// **Why the rule is about the function and not the reader** (`/pending 717`): the guard this replaces looked for a
// `bytes.NewReader` built in the same function as the `api` call, and could not see a reader built in one function
// and handed to pdfcpu from another, nor one of any other constructor. Banning the reader-taking functions themselves
// makes where a reader came from irrelevant: there is no door left that hands pdfcpu a reader of a PDF.
func TestEveryValidatingReadRoutesThroughTheDoor(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bannedAPI := map[string]bool{"ReadAndValidate": true, "ReadValidateAndOptimize": true, "ValidateContext": true, "OptimizeContext": true}
	bannedPdfcpu := map[string]bool{"OptimizeXRefTable": true, "OptimizeContext": true}
	readsItself := apiFunctionsThatReadADocument(t)
	// Stimulus: the set is read from pdfcpu's source, not an empty set every call passes.
	for _, want := range []string{"Bookmarks", "MergeRaw", "NUp", "Encrypt", "ExtractImages", "PageDims", "Create", "ReadContext", "MultiFillForm"} {
		if _, ok := readsItself[want]; !ok {
			t.Fatalf("setup: pdfcpu's api.%s is not among the functions found to read a document (%d found) — the parse "+
				"is not reading pdfcpu's signatures", want, len(readsItself))
		}
	}
	scanned, apiFiles := 0, 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir // .git, and .claude/worktrees: other checkouts, not this module
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
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		scanned++
		apiName, pdfcpuName := "", ""
		for _, im := range file.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			local := p[strings.LastIndex(p, "/")+1:]
			if im.Name != nil {
				local = im.Name.Name
			}
			switch p {
			case "github.com/pdfcpu/pdfcpu/pkg/api":
				apiName = local
			case "github.com/pdfcpu/pdfcpu/pkg/pdfcpu":
				pdfcpuName = local
			case "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/validate":
				t.Errorf("%s imports pdfcpu's validate package — validate through pdfread.Validated", rel)
			}
		}
		if apiName == "" && pdfcpuName == "" {
			return nil
		}
		apiFiles++
		isSel := func(e ast.Expr, pkg string) (string, bool) {
			sel, ok := e.(*ast.SelectorExpr)
			if !ok || pkg == "" {
				return "", false
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok || x.Name != pkg {
				return "", false
			}
			return sel.Sel.Name, true
		}
		// Rule 2's calls first: a call to a function that reads a document itself is allowed only with nil at every
		// reader position, and its selector is then judged; any other reference to one is refused below.
		judged := map[*ast.SelectorExpr]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name, ok := isSel(sel, apiName)
			if !ok {
				return true
			}
			pos, reads := readsItself[name]
			if !reads || name == "ReadContext" {
				return true
			}
			judged[sel] = true
			for _, i := range pos {
				if i < 0 || i >= len(c.Args) {
					t.Errorf("%s:%d: api.%s reads a document from disk itself, past the reference door and the optimize "+
						"budget — read the bytes and call its restatement in pdfread (apiread.go)", rel, fset.Position(c.Pos()).Line, name)
					break
				}
				if id, ok := c.Args[i].(*ast.Ident); !ok || id.Name != "nil" {
					t.Errorf("%s:%d: api.%s reads the document it is handed itself, past the reference door and the "+
						"optimize budget — call its restatement in pdfread (apiread.go), or add one", rel,
						fset.Position(c.Pos()).Line, name)
					break
				}
			}
			return true
		})
		// **Every reference, not only a call, and the whole file, not only function bodies** (the review of
		// /pending 675/714): `var read = api.ReadAndValidate` at package level, or `f := api.X; f(...)`, reached
		// pdfcpu's validator past a guard that inspected call sites inside functions. And every `api.*File`
		// function reads a path from disk through its own validating read.
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if name, ok := isSel(sel, apiName); ok {
				_, reads := readsItself[name]
				switch {
				case bannedAPI[name] || strings.HasSuffix(name, "File"):
					t.Errorf("%s:%d: names api.%s — read through pdfread (Validated, ReadOptimized), optimize "+
						"through pdfread.Optimize", rel, fset.Position(sel.Pos()).Line, name)
				case reads && name != "ReadContext" && !judged[sel]:
					t.Errorf("%s:%d: names api.%s other than in a call — it reads a document itself; use its "+
						"restatement in pdfread (apiread.go)", rel, fset.Position(sel.Pos()).Line, name)
				}
			}
			if name, ok := isSel(sel, pdfcpuName); ok && bannedPdfcpu[name] {
				t.Errorf("%s:%d: names pdfcpu.%s — optimize through pdfread.Optimize", rel, fset.Position(sel.Pos()).Line, name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 200 || apiFiles < 20 {
		t.Fatalf("scanned %d source files, %d using pdfcpu's api — the guard is not reading the module", scanned, apiFiles)
	}
}

// apiFunctionsThatReadADocument is every exported function of pdfcpu's `api` package, in the source the module builds
// against, that reads a document itself: each with the argument positions of its reader parameters (`io.ReadSeeker`,
// `[]io.ReadSeeker`), and one that takes a path (`inFile…`) with -1, since no argument makes it safe.
func apiFunctionsThatReadADocument(t *testing.T) map[string][]int {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/pdfcpu/pdfcpu").Output()
	if err != nil {
		t.Fatalf("setup: go list could not locate pdfcpu's source: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(strings.TrimSpace(string(out)), "pkg", "api", "*.go"))
	isReader := func(e ast.Expr) bool {
		if a, ok := e.(*ast.ArrayType); ok {
			e = a.Elt
		}
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		x, ok := sel.X.(*ast.Ident)
		return ok && x.Name == "io" && strings.HasPrefix(sel.Sel.Name, "ReadSeek")
	}
	set := map[string][]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !fd.Name.IsExported() {
				continue
			}
			var pos []int
			path, i := false, 0
			for _, field := range fd.Type.Params.List {
				names := field.Names
				if len(names) == 0 {
					names = []*ast.Ident{nil}
				}
				for _, n := range names {
					if isReader(field.Type) {
						pos = append(pos, i)
					}
					if n != nil && strings.HasPrefix(n.Name, "inFile") {
						path = true
					}
					i++
				}
			}
			switch {
			case path:
				set[fd.Name.Name] = []int{-1}
			case len(pos) > 0:
				set[fd.Name.Name] = pos
			}
		}
	}
	return set
}
