package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestOnlyThePostedBytesRoutesSkipTheLostUpdateGuard — /pending 646 #3.
//
// `commitMutation` refuses a stale snapshot only when its base is a `snapshotBase`, so a handler
// that read the document and passed `postedBase` would silently go back to last-writer-wins with
// every test green. The two routes that work from a CLIENT'S posted PDF are the named exemption;
// every other production commit must state its base as a snapshot.
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
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "commitMutation" || len(call.Args) < 2 {
					return true
				}
				seen++
				base := ""
				if c, ok := call.Args[1].(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok {
						base = id.Name
					}
				}
				pos := fset.Position(call.Pos())
				switch {
				case base == "snapshotBase":
				case base == "postedBase" && exempt[fn.Name.Name]:
					posted[fn.Name.Name] = true
				default:
					t.Errorf("%s:%d (%s): commitMutation's base is %q — a handler that read the "+
						"document passes snapshotBase(before), or a concurrent operation's edit is "+
						"silently lost; postedBase is only for the named posted-bytes routes",
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
			t.Errorf("%s is exempt but no longer commits posted bytes — drop the exemption", name)
		}
	}
}
