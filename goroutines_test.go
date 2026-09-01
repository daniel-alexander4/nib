package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryDetachedGoroutineIsRecovered is the census, made into a guard — repo-wide.
//
// # Why it lives at the root and not in internal/server
//
// It used to live in `internal/server/racelifetime_test.go`, scoped by `pkgs["server"]`. The
// law it enforces is not a law about `internal/server`: `safe.Recover`'s own doc says a panic
// in ANY goroutine takes the desktop process down with the user's unsaved documents. A guard
// that checks one package of twelve is a rule with one door of twelve — and the cost was paid,
// not theorised: `internal/udpmux`'s `readLoop`, the shared socket's sole reader of untrusted
// inbound datagrams, shipped with no recover until the P05 close found it by hand (v1.117.107).
// The guard could not have seen it.
//
// ADR-009 — a rule gets ONE door and the guard checks the door — is also why the
// internal/server copy was DELETED rather than left beside this. A root `package nib` test
// cannot import that package's helpers, so keeping both would have guaranteed two
// implementations of one predicate, which is the defect ADR-009 was written from.
//
// # What it asserts, and the two doors
//
// A goroutine is detached through `go` **or** through `time.AfterFunc`, whose callback runs on
// its own goroutine with no caller to unwind into. A guard that walked only `go` statements
// would have reported full coverage over a tree where two `AfterFunc` callbacks — one of them
// closing a channel on the untrusted-datagram path — had no recover at all. Both doors are
// walked here.
//
// It asserts the ROUTING, not the label text: eight sites checked for agreement say nothing
// about a ninth added without one, and the label is not the property.
//
// # What it cannot prove, said plainly
//
// Callee resolution is by NAME within the package, not by type. `go x.f()` passes if any
// function or method named `f` in that package recovers itself, even if `x`'s own `f` does
// not. Closing that needs go/types over a built package; this is a source scan. The shape it
// reliably catches is the one that actually shipped — a goroutine with no recover anywhere
// near it — and a same-named-method false pass has never occurred here.
//
// Test files are excluded deliberately, and it is not an oversight to fix: `safe.Recover` in a
// test goroutine swallows the panic that IS the failure signal.
//
// `net/http` handler goroutines are out of scope by construction — they are not `go`
// statements in this source, and `http.Server` already recovers per-connection panics.
func TestEveryDetachedGoroutineIsRecovered(t *testing.T) {
	fset := token.NewFileSet()

	// Discover the population; never list it. A hand-listed package set is this same defect
	// one level up — a package added later is invisible exactly the way internal/udpmux was.
	byDir := map[string][]*ast.File{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			// `.claude` holds git WORKTREES a second session creates inside this repo (the
			// rule in CLAUDE.md), each a full copy of the tree. It is gitignored, so it is not
			// source — and walking it DOUBLES every census this guard takes. Because the floors
			// below are lower bounds, that direction passes silently, which is the worse one.
			case ".git", ".claude", "node_modules", "web", "docs", "test":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		dir := filepath.Dir(path)
		byDir[dir] = append(byDir[dir], f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// STIMULUS for the walk itself: a walk that parsed nothing gives every check below a
	// clean bill. Twelve internal packages plus cmd/nib is the floor the tree already meets.
	if len(byDir) < 13 {
		t.Fatalf("setup: only %d source directories parsed — this guard is not walking the repo", len(byDir))
	}

	var goStmts, viaCallee, viaLiteral, afterFuncs int
	for dir, files := range byDir {
		// Which functions in THIS package recover themselves, so a bare `go f()` can be
		// resolved one hop. A bare call has nowhere to hang a defer, so either the callee
		// recovers itself or the launch is wrapped in a literal that does.
		recovers := map[string]bool{}
		for _, f := range files {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Body == nil || len(fd.Body.List) == 0 {
					continue
				}
				if def, ok := fd.Body.List[0].(*ast.DeferStmt); ok && isSafeRecoverCall(def.Call) {
					recovers[fd.Name.Name] = true
				}
			}
		}

		for _, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.GoStmt:
					goStmts++
					pos := fset.Position(node.Pos())
					lit, ok := node.Call.Fun.(*ast.FuncLit)
					if !ok {
						// A bare call — including `go x.f()` where `f` is a func FIELD
						// rather than a method. A field has no FuncDecl to resolve to, so
						// it is reported rather than silently resolved: that is exactly
						// how internal/p2p's accept loop hid.
						if calleeRecoversIn(node.Call.Fun, recovers) {
							viaCallee++
							return true
						}
						t.Errorf("%s:%d launches `go %s(...)`, and no function of that name in "+
							"%s defers safe.Recover as its first statement. A bare call has "+
							"nowhere to hang a defer, so either the callee recovers itself or "+
							"the launch is wrapped in a function literal that does.",
							pos.Filename, pos.Line, types.ExprString(node.Call.Fun), dir)
						return true
					}
					if len(lit.Body.List) == 0 {
						t.Errorf("%s:%d launches an empty goroutine", pos.Filename, pos.Line)
						return true
					}
					first, ok := lit.Body.List[0].(*ast.DeferStmt)
					if !ok || !isSafeRecoverCall(first.Call) {
						t.Errorf("%s:%d launches a goroutine whose FIRST statement is not "+
							"`defer safe.Recover(...)`. An unrecovered panic in ANY goroutine "+
							"takes the desktop process down with the user's unsaved documents, "+
							"and safe.Recover's doc requires it at the very top so the "+
							"goroutine's other defers still run as the stack unwinds.",
							pos.Filename, pos.Line)
						return true
					}
					viaLiteral++
				case *ast.CallExpr:
					if !isTimeAfterFunc(node) || len(node.Args) < 2 {
						return true
					}
					afterFuncs++
					pos := fset.Position(node.Pos())
					lit, ok := node.Args[1].(*ast.FuncLit)
					if !ok {
						if calleeRecoversIn(node.Args[1], recovers) {
							return true
						}
						t.Errorf("%s:%d hands time.AfterFunc a callback that does not defer "+
							"safe.Recover first. AfterFunc runs it on its own goroutine, so a "+
							"panic there has no caller to unwind into either.",
							pos.Filename, pos.Line)
						return true
					}
					if len(lit.Body.List) == 0 {
						return true // an empty callback cannot panic
					}
					first, ok := lit.Body.List[0].(*ast.DeferStmt)
					if !ok || !isSafeRecoverCall(first.Call) {
						t.Errorf("%s:%d hands time.AfterFunc a callback whose first statement "+
							"is not `defer safe.Recover(...)`. It runs on its own goroutine: "+
							"the same law as `go`, through the other door.",
							pos.Filename, pos.Line)
					}
				}
				return true
			})
		}
	}

	// STIMULUS, four directions. Each one is a way this guard goes quietly blind: a walk that
	// found nothing, a repo that stopped using goroutines, and either resolution arm carried
	// forever without ever running — wrong the day somebody relies on it.
	if goStmts < 33 {
		t.Errorf("found only %d `go` statements repo-wide; the census at v1.117.113 was 33, "+
			"so this walk is not seeing the whole tree", goStmts)
	}
	if afterFuncs < 2 {
		t.Errorf("found only %d time.AfterFunc call sites; the census at v1.117.113 was 2. "+
			"If the last one was deliberately removed, delete this door rather than leaving "+
			"it unexercised", afterFuncs)
	}
	if viaCallee == 0 {
		t.Error("no `go f()` resolved through a self-recovering callee, so that arm of this " +
			"guard ran against nothing. If the last such call site was deliberately rewritten " +
			"as a literal, delete the resolution rather than leaving it unexercised.")
	}
	if viaLiteral == 0 {
		t.Error("no `go func(){...}()` was checked, so the literal arm ran against nothing.")
	}
}

// calleeRecoversIn reports whether a bare `go f()` / `go x.f()` names something in the same
// package that defers safe.Recover as its own first statement. One hop, not a call graph: a
// goroutine two frames from its recover is a shape nobody should write without saying why.
func calleeRecoversIn(fun ast.Expr, recovers map[string]bool) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return recovers[f.Name]
	case *ast.SelectorExpr:
		return recovers[f.Sel.Name]
	}
	return false
}

// isSafeRecoverCall reports whether a call is `safe.Recover(...)`, matched on the selector
// rather than on the rendered text so a renamed import does not silently pass.
func isSafeRecoverCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Recover" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "safe"
}

// isTimeAfterFunc reports whether a call is `time.AfterFunc(...)`.
func isTimeAfterFunc(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "AfterFunc" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "time"
}
