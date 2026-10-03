package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// docRouteUnlockedExempt names the routes whose handlers reach the document registry and are NOT
// behind `requireUnlocked`, each with why. Empty is the expected state; an entry is a decision.
var docRouteUnlockedExempt = map[string]string{}

// TestEveryDocumentRouteIsBehindTheUnlock — /pending 805 (R5). ADR-054's census holds that every
// route needs the SESSION; nothing held that a route serving a DOCUMENT also needs the vault
// UNLOCKED. A document route registered with `requireSession` alone would serve the open
// documents to a page sitting on the unlock screen, and every existing guard would stay green.
//
// "Serves a document" is decided by the call graph, not by a list: a handler that reaches
// `resolveDoc`, `docFor`, `activeDoc` or `docBytes` — directly or through any chain of this
// package's own methods and functions — is a document route.
func TestEveryDocumentRouteIsBehindTheUnlock(t *testing.T) {
	fset := token.NewFileSet()
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]map[string]bool{} // function or method name -> names it calls
	var handlerSrc string
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := fd.Name.Name
			if fd.Name.Name == "Handler" && fd.Recv != nil {
				b, _ := os.ReadFile(n)
				handlerSrc = string(b[fset.Position(fd.Body.Pos()).Offset:fset.Position(fd.Body.End()).Offset])
			}
			set := calls[name]
			if set == nil {
				set = map[string]bool{}
				calls[name] = set
			}
			ast.Inspect(fd.Body, func(x ast.Node) bool {
				c, ok := x.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fn := c.Fun.(type) {
				case *ast.Ident:
					set[fn.Name] = true
				case *ast.SelectorExpr:
					if id, ok := fn.X.(*ast.Ident); ok && id.Name == "s" {
						set[fn.Sel.Name] = true
					}
				}
				return true
			})
		}
	}
	seeds := []string{"resolveDoc", "docFor", "activeDoc", "docBytes"}
	for _, s := range seeds {
		if _, ok := calls[s]; !ok {
			t.Fatalf("stimulus: %s is not declared in this package — the census would call nothing a document route", s)
		}
	}
	reaches := map[string]bool{}
	for _, s := range seeds {
		reaches[s] = true
	}
	for changed := true; changed; {
		changed = false
		for fn, set := range calls {
			if reaches[fn] {
				continue
			}
			for callee := range set {
				if reaches[callee] {
					reaches[fn] = true
					changed = true
					break
				}
			}
		}
	}

	if handlerSrc == "" {
		t.Fatal("stimulus: could not read the body of (*Server).Handler")
	}
	route := regexp.MustCompile(`mux\.HandleFunc\("([^"]+)",\s*(.+)\)\s*$`)
	last := regexp.MustCompile(`s\.(handle\w+)\)*$`)
	docRoutes, bad := 0, []string{}
	for _, line := range strings.Split(handlerSrc, "\n") {
		m := route.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		h := last.FindStringSubmatch(m[2])
		if h == nil || !reaches[h[1]] {
			continue
		}
		docRoutes++
		if strings.Contains(m[2], "requireUnlocked(") {
			continue
		}
		if _, ok := docRouteUnlockedExempt[m[1]]; ok {
			continue
		}
		bad = append(bad, m[1]+" → "+h[1])
	}
	if docRoutes < 30 {
		t.Fatalf("stimulus: only %d document route(s) found — the census is not reading the route table or the call graph", docRoutes)
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("%s reaches the open documents and is not behind requireUnlocked — a page on the unlock "+
			"screen would be served them. Wrap it, or name it in docRouteUnlockedExempt with the reason", b)
	}
}
