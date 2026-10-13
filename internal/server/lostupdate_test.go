package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestOnlyThePostedBytesRoutesSkipTheLostUpdateGuard — /pending 646 #3, extended by /pending 792.
//
// `commitMutation` refuses a stale snapshot only when its base is a `snapshotBase`, so a handler
// that read the document and passed anything else would silently go back to last-writer-wins with
// every test green. The two routes that work from a CLIENT'S posted PDF are the named exemption,
// and since ADR-146 they do not skip the guard either: each builds its base with `echoedBase`, which
// carries the revision the window says it built on. A bare `postedBase` is for tests — in production
// only `echoedBase` may call it, and only the two named routes may call `echoedBase`.
func TestOnlyThePostedBytesRoutesSkipTheLostUpdateGuard(t *testing.T) {
	exempt := map[string]bool{"handlePages": true, "handleOutlineSet": true}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	seen, posted := 0, map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				pos := fset.Position(call.Pos())
				if id, ok := call.Fun.(*ast.Ident); ok {
					switch {
					case id.Name == "postedBase" && fn.Name.Name != "echoedBase":
						t.Errorf("%s:%d (%s) builds a postedBase — a base with no word on what it was built "+
							"on is for tests; a route posts through echoedBase (ADR-146)", pos.Filename, pos.Line, fn.Name.Name)
					case id.Name == "echoedBase" && exempt[fn.Name.Name]:
						posted[fn.Name.Name] = true
					case id.Name == "echoedBase":
						t.Errorf("%s:%d (%s) builds an echoedBase — only the named posted-bytes routes do; a "+
							"handler that read the document passes snapshotBase(before)", pos.Filename, pos.Line, fn.Name.Name)
					}
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "commitMutation" || len(call.Args) < 2 {
					return true
				}
				seen++
				base := ""
				switch a := call.Args[1].(type) {
				case *ast.CallExpr:
					if id, ok := a.Fun.(*ast.Ident); ok {
						base = id.Name
					}
				case *ast.Ident:
					base = "var " + a.Name
				}
				switch {
				case base == "snapshotBase":
				case base == "var base" && exempt[fn.Name.Name]:
					// the value echoedBase returned, checked above
				default:
					t.Errorf("%s:%d (%s): commitMutation's base is %q — a handler that read the "+
						"document passes snapshotBase(before), or a concurrent operation's edit is "+
						"silently lost; only the named posted-bytes routes pass an echoedBase",
						pos.Filename, pos.Line, fn.Name.Name, base)
				}
				return true
			})
		}
	}
	if seen < 10 {
		t.Errorf("found %d commitMutation calls, expected at least 10 — this guard has gone blind", seen)
	}
	for name := range exempt {
		if !posted[name] {
			t.Errorf("%s is exempt but no longer builds an echoedBase — drop the exemption", name)
		}
	}
}
