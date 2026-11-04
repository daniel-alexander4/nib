package docmodel

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

// TestEveryWriterReadsModelTextThroughClean — ADR-009 for ADR-193 (/pending 941 #22).
//
// `Clean` is the one door text passes through on its way to a writer (PLAN-import-export Y9), and until this guard it was
// a convention: a seventh writer, or a new line in an old one, that wrote `s.Text` straight into its output would have
// compiled, passed every test with clean input, and put a NUL in an XML 1.0 part. The guard checks the door, not the
// text each writer prints: every READ of a model text field — a span's `Text`, an image's `Alt`, a block's `Marker`, a
// `Title` — in a non-test file of `internal/docwrite/*` must sit lexically inside a call to a door (below), or the
// function holding it must be a named exemption whose body is verified to call `Clean` or `OneLine` itself. Links are
// not covered here: a destination is judged by `xmlw.SafeLink` and each writer's own encoder, not by `Clean`.
//
// A door is `Clean` and `OneLine` (docmodel's), plus the writers' own one-line wrappers `Esc`, `esc` and `text` — each of
// which is VERIFIED to call `Clean` or `OneLine` in its own body, so a door that stopped cleaning turns this red — plus
// `Numbered`, which only classifies a marker and writes nothing. The scan is by name, not by type: a field called
// `Text` on an unrelated struct is caught too and earns a row, which is the cheap direction to be wrong in.
func TestEveryWriterReadsModelTextThroughClean(t *testing.T) {
	// Keyed `<file>:<enclosing function>`; the value is the reason, and the function is verified to call a cleaner.
	exempt := map[string]string{
		"internal/docwrite/html/html.go:plainLine": "joins a heading's spans and returns `docmodel.OneLine` of the join: the cleaning is one call over the whole line",
		"internal/docwrite/text/text.go:plain":     "the same, for the plain-text writer: spans joined, then `docmodel.OneLine`",
		"internal/docwrite/markdown/markdown.go:spans": "merges neighbouring runs of one style by concatenating their raw text, then every piece is " +
			"`docmodel.Clean`ed / `OneLine`d before it is measured or written",
		"internal/docwrite/epub/epub.go:Write": "reads `html.Heading.Text`, which `html.plainLine` produced with `OneLine`, and writes it through `xmlw.Esc`",
	}
	doors := map[string]bool{"Clean": true, "OneLine": true, "Esc": true, "esc": true, "text": true, "Numbered": true}
	// Doors defined under docwrite: each must call a cleaner in its own body.
	local := map[string]bool{"Esc": true, "esc": true, "text": true}
	watched := map[string]bool{"Text": true, "Alt": true, "Marker": true, "Title": true}

	root := filepath.Join("..", "docwrite")
	dirs, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var writers []string
	for _, d := range dirs {
		if d.IsDir() && d.Name() != "wtest" { // wtest is test support, not a writer
			writers = append(writers, d.Name())
		}
	}
	if len(writers) < 6 {
		t.Fatalf("found writers %v: the census cannot see the six it exists for", writers)
	}

	var violations []string
	doored := map[string]int{}     // writer -> reads that sit inside a door
	hitRow := map[string]bool{}    // exemption rows used
	doorBodies := map[string]int{} // local door definitions seen, by name
	fset := token.NewFileSet()
	for _, w := range writers {
		files, _ := filepath.Glob(filepath.Join(root, w, "*.go"))
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			rel := "internal/docwrite/" + w + "/" + filepath.Base(path)
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				if local[fd.Name.Name] {
					doorBodies[fd.Name.Name]++
					if !callsAny(fd.Body, "Clean", "OneLine") {
						violations = append(violations, fmt.Sprintf("%s: the door `%s` no longer calls Clean or OneLine", rel, fd.Name.Name))
					}
				}
				key := rel + ":" + fd.Name.Name
				var stack []ast.Node
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					if n == nil {
						stack = stack[:len(stack)-1]
						return true
					}
					stack = append(stack, n)
					sel, ok := n.(*ast.SelectorExpr)
					if !ok || !watched[sel.Sel.Name] {
						return true
					}
					if len(stack) > 1 { // `xmlw.Title(m)` is a call of a function, not a read of a field
						if c, ok := stack[len(stack)-2].(*ast.CallExpr); ok && c.Fun == n {
							return true
						}
					}
					for _, anc := range stack[:len(stack)-1] {
						if c, ok := anc.(*ast.CallExpr); ok && doors[calleeName(c)] {
							doored[w]++
							return true
						}
					}
					if _, ok := exempt[key]; ok {
						hitRow[key] = true
						return true
					}
					violations = append(violations, fmt.Sprintf("%s: reads `.%s` outside a door (%s)", fset.Position(sel.Pos()), sel.Sel.Name, key))
					return true
				})
			}
		}
	}
	// The rows are claims about code: each function must exist, be used, and clean.
	for key, why := range exempt {
		if why == "" {
			violations = append(violations, key+": an exemption with no reason")
		}
		if !hitRow[key] {
			violations = append(violations, key+": an exemption no read needs any more — remove the row")
			continue
		}
		file, fn, _ := strings.Cut(key, ":")
		f, err := parser.ParseFile(fset, filepath.Join("..", "..", file), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		cleans := false
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == fn && fd.Body != nil {
				cleans = callsAny(fd.Body, "Clean", "OneLine", "Esc", "esc", "text")
			}
		}
		if !cleans {
			violations = append(violations, key+": exempt, and the function calls no cleaner")
		}
	}
	// Positive controls: the scan can see a read and a door, in every writer and in every local door.
	for _, w := range writers {
		if w == "xmlw" {
			continue // xmlw holds the shared helpers; it has no model text of its own to write
		}
		if doored[w] == 0 && !hasRow(exempt, w) {
			violations = append(violations, "internal/docwrite/"+w+": no read of model text through a door was seen: the writer, or this scan, is blind")
		}
	}
	for name := range local {
		if doorBodies[name] == 0 {
			violations = append(violations, "the door `"+name+"` is not defined under docwrite: this scan lists a door that does not exist")
		}
	}
	sort.Strings(violations)
	for _, v := range violations {
		t.Error(v)
	}
}

func hasRow(exempt map[string]string, writer string) bool {
	for k := range exempt {
		if strings.Contains(k, "/docwrite/"+writer+"/") {
			return true
		}
	}
	return false
}

// calleeName is the last identifier of a call's function: `docmodel.Clean(x)` and `Clean(x)` are both "Clean".
func calleeName(c *ast.CallExpr) string {
	switch f := c.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func callsAny(body ast.Node, names ...string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			for _, nm := range names {
				if calleeName(c) == nm {
					found = true
				}
			}
		}
		return !found
	})
	return found
}
