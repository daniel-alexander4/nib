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

// moduleFile is one parsed non-test Go file of the module outside this package.
type moduleFile struct {
	rel  string
	fset *token.FileSet
	file *ast.File
	door string // the local name `nib/internal/pdfread` is imported under, or ""
}

// moduleFiles parses every non-test Go file in the module outside this package — the population both censuses
// below walk.
func moduleFiles(t *testing.T) []moduleFile {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var out []moduleFile
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
		out = append(out, moduleFile{rel: filepath.ToSlash(rel), fset: fset, file: file, door: door})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestEveryPageContentReadRoutesThroughTheDoor — ADR-056 under ADR-009: a divided `/Contents` is read correctly
// only if nothing reads a page's content around `pdfread.PageContent`. An AST census over every non-test Go file
// in the module outside this package, with two findings:
//
//   - a call to any method named `PageContent` — pdfcpu's, on a `*model.Context` or an `*XRefTable`;
//   - a READ of the key `/Contents` — `x["Contents"]` other than as an assignment's target, or `.Find("Contents")` —
//     because a reader that walks a page's streams itself never calls a method of that name, and so was invisible
//     here (`resourceprune.go` decoded every stream of a page one by one, unmarked — `/pending 728`).
//
// Either is excused only by a marker on its line: `//pagecontent:exempt <name>`, where the name is one of the
// exemptions listed here at the file it is declared for, or — for a key read only — `//pagecontent:key <reason>`,
// which says the key read is not a page's content being decoded (an annotation's text, a signature's blob, an
// object number). Calls to the door itself (`pdfread.PageContent`, through the import's local name) are counted, so
// a census that stopped seeing them reads red, not green.
func TestEveryPageContentReadRoutesThroughTheDoor(t *testing.T) {
	exempt := map[string]string{
		// `ContentDigest` was an exemption until ADR-080: rule 5 reads through the door, and rule 4 — kept to check v4
		// records — reads `PageContentAsPdfcpu`, the door's own bounded copy of pdfcpu's join. The checker reads
		// through the door since /pending 719 measured veraPDF's join, and the n-up carries since /pending 728.
		// Names are read per stream — the door's own meaning, since it separates only at stream boundaries and
		// a per-stream tokenize treats every boundary as one — and memoized per object across every kept page.
		"resourceprune-names": "internal/pdfops/resourceprune.go",
		// Edits the prefix pdfcpu's stamp wrote into the FIRST stream; it reads no join.
		"stamp-prefix": "internal/pdfops/stamprotation.go",
	}
	routed, keyReads, seenExempt := 0, 0, map[string]int{}
	for _, mf := range moduleFiles(t) {
		rel, fset := mf.rel, mf.fset
		type mark struct{ kind, name string }
		marks := map[int]mark{}
		for _, cg := range mf.file.Comments {
			for _, c := range cg.List {
				for _, kind := range []string{"exempt", "key"} {
					if rest, ok := strings.CutPrefix(c.Text, "//pagecontent:"+kind); ok {
						name, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
						marks[fset.Position(c.Pos()).Line] = mark{kind, name}
					}
				}
			}
		}
		excuse := func(pos token.Pos, what string, keyRead bool) {
			line := fset.Position(pos).Line
			m, marked := marks[line]
			switch {
			case !marked:
				t.Errorf("%s:%d %s — read it through pdfread.PageContent (ADR-056), or mark the line", rel, line, what)
			case m.kind == "key" && keyRead:
				if m.name == "" {
					t.Errorf("%s:%d is marked //pagecontent:key with no reason", rel, line)
				}
				keyReads++
			case m.kind == "key":
				t.Errorf("%s:%d is marked //pagecontent:key, but it calls PageContent — that is a content read", rel, line)
			case exempt[m.name] != rel:
				t.Errorf("%s:%d is marked //pagecontent:exempt %q, which is not an exemption declared for this file "+
					"in TestEveryPageContentReadRoutesThroughTheDoor", rel, line, m.name)
			default:
				seenExempt[m.name]++
			}
		}
		assigned := map[ast.Expr]bool{}
		ast.Inspect(mf.file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				for _, l := range n.Lhs {
					assigned[l] = true
				}
			case *ast.IndexExpr:
				if isContentsLit(n.Index) && !assigned[n] {
					excuse(n.Pos(), "reads a page's /Contents key itself", true)
				}
			case *ast.CallExpr:
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch {
				case sel.Sel.Name == "Find" && len(n.Args) == 1 && isContentsLit(n.Args[0]):
					excuse(n.Pos(), "reads a page's /Contents key itself", true)
				case sel.Sel.Name == "PageContent":
					if id, isID := sel.X.(*ast.Ident); isID && mf.door != "" && id.Name == mf.door {
						routed++
						return true
					}
					excuse(n.Pos(), "reads a page's content through pdfcpu's PageContent, which joins a /Contents "+
						"array with no separator", false)
				}
			}
			return true
		})
	}
	// The stimulus: the census must be SEEING the door's callers and the key's readers, or a walk that parsed
	// nothing passes.
	if routed < 15 {
		t.Errorf("the census saw %d calls to pdfread.PageContent; at /pending 728 there were 18 — it is not reading the tree", routed)
	}
	if keyReads < 4 {
		t.Errorf("the census saw %d marked /Contents key reads; at /pending 728 there were 4 — it is not reading the tree", keyReads)
	}
	// Each exemption excuses exactly ONE site: a name that matched none is stale and excuses whatever lands there
	// next, and one that matched two is excusing a site nobody declared.
	for name, file := range exempt {
		if n := seenExempt[name]; n != 1 {
			t.Errorf("the exemption %q (%s) matched %d sites, want exactly 1", name, file, n)
		}
	}
}

func isContentsLit(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == `"Contents"`
}

// joinWriters are pdfcpu's operations that read a page through pdfcpu's own bare join of `/Contents` and write
// what they read into the document — `NUpFromPDF` (nup.go:328), `Resize` (resize.go:276), `CutPage`
// (cut.go:175/263/353), `Zoom` (zoom.go:123) — with the `api` entry points that reach them.
var joinWriters = map[string]map[string]bool{
	"pdfcpu": {"NUpFromPDF": true, "Resize": true, "CutPage": true, "Zoom": true},
	"api": {"NUp": true, "NUpFile": true, "Resize": true, "ResizeFile": true, "Cut": true, "CutFile": true,
		"Zoom": true, "ZoomFile": true, "Booklet": true, "BookletFile": true, "Poster": true, "PosterFile": true,
		"NDown": true, "NDownFile": true},
}

// TestEveryPdfcpuJoinWriterIsHandedSeparatedContents — ADR-084 under ADR-009. pdfcpu's page operations write their
// own bare join of a divided page into the output, fusing `(A) Tj` | `ET` into the unknown operator `TjET`, and
// `pdfread.SeparateContents` is the one door that prevents it. Every call to one of `joinWriters` must sit in a
// function that calls the door EARLIER in its body; the `api` entry points read the document themselves and so
// cannot be handed a separated context at all, and are findings wherever they appear.
func TestEveryPdfcpuJoinWriterIsHandedSeparatedContents(t *testing.T) {
	seen := 0
	for _, mf := range moduleFiles(t) {
		for _, decl := range mf.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			doorAt := token.NoPos
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if mf.door != "" && pkg.Name == mf.door && sel.Sel.Name == "SeparateContents" {
					if doorAt == token.NoPos {
						doorAt = call.Pos()
					}
					return true
				}
				if !joinWriters[pkg.Name][sel.Sel.Name] {
					return true
				}
				seen++
				line := mf.fset.Position(call.Pos()).Line
				switch {
				case pkg.Name == "api":
					t.Errorf("%s:%d calls api.%s, which reads the document itself and writes pdfcpu's bare join of a "+
						"divided page; read the context, pdfread.SeparateContents it, and call the pdfcpu operation "+
						"(ADR-084)", mf.rel, line, sel.Sel.Name)
				case doorAt == token.NoPos:
					t.Errorf("%s:%d calls pdfcpu.%s without pdfread.SeparateContents earlier in %s — a divided page "+
						"comes out fused (ADR-084)", mf.rel, line, sel.Sel.Name, fn.Name.Name)
				}
				return true
			})
		}
	}
	// The stimulus: at ADR-084 three sites call a join writer (n-up, resize, cut); seeing none is a census that
	// stopped reading.
	if seen < 3 {
		t.Errorf("the census saw %d calls to pdfcpu's join writers; at ADR-084 there were 3 — it is not reading the tree", seen)
	}
}
