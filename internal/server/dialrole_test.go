package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// TestEveryDialDeclaresItsRole — ADR-028's call-site half. See
// `p2p.TestEveryVerificationInitiatorIsAKnownRoleWriter` for the other half and for the defect
// both were written from: the role frame reached `Initiate` and `SendDocument` and missed `Carry`,
// so the relay's baton hop put a 32-byte commitment where a one-byte role was expected.
//
// **The two guards are deliberately split across the two packages.** p2p knows which verbs
// initiate; only the server knows where they are called. Either alone is satisfiable while the
// defect stands: p2p's list can be complete with a call site that declares nothing, and this one
// can be clean while a fourth verb nobody listed is dialling.
func TestEveryDialDeclaresItsRole(t *testing.T) {
	// The initiating verbs. Kept in step with p2p's `known` map by that package's own guard, which
	// fails when a verb initiates and is not listed there.
	verbs := map[string]bool{"Initiate": true, "Carry": true, "SendDocument": true}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	sites := 0
	var bare, late []string
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			for _, d := range file.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				// Every enclosing function that calls an initiating verb must also call
				// WriteRole. Function-scoped rather than statement-scoped on purpose: the hop
				// path chooses its verb in a closure and declares the role ABOVE the branch,
				// which is the correct shape and which a line-ordering check would refuse.
				var calls []string
				var callPos []int
				writes := false
				firstWrite := -1
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					pkgIdent, ok := sel.X.(*ast.Ident)
					if !ok || pkgIdent.Name != "p2p" {
						return true
					}
					line := fset.Position(call.Pos()).Line
					if sel.Sel.Name == "WriteRole" {
						writes = true
						if firstWrite < 0 || line < firstWrite {
							firstWrite = line
						}
					}
					if verbs[sel.Sel.Name] {
						sites++
						calls = append(calls, sel.Sel.Name+" ("+path+":"+itoa(line)+")")
						callPos = append(callPos, line)
					}
					return true
				})
				if len(calls) > 0 && !writes {
					bare = append(bare, calls...)
					continue
				}
				// ── And it must come FIRST, which is the half that caught nothing until it did ──
				//
				// The function-scoped rule above is satisfied by a role written on ONE arm of a
				// branch — which is precisely the defect that shipped: `WriteRole` sat inside the
				// `Initiate` arm while `Carry` returned above it, so the baton hop declared
				// nothing. A positional rule catches it: the declaration must precede every verb
				// it is supposed to cover, because a verb that can be reached without passing the
				// write is a verb that can dial undeclared.
				for i, at := range callPos {
					if firstWrite >= 0 && at < firstWrite {
						late = append(late, calls[i])
					}
				}
			}
		}
	}

	// STIMULUS. A scan that resolved no call sites agrees with the assertion below. Three is the
	// count at the time of writing — Initiate and Carry share one enclosing function, plus the two
	// SendDocument sites — and it is a floor on a tree that only grows.
	if sites < 3 {
		t.Fatalf("found only %d dial site(s) across the package, want at least 3 — the scan is "+
			"not resolving `p2p.<verb>(` calls and a clean result says nothing", sites)
	}

	if len(late) > 0 {
		t.Errorf("%d dial site(s) are reached BEFORE their function declares a role (ADR-028):\n  %s"+
			"\n\nA role written after a verb — or inside one arm of a branch the verb returns "+
			"from — covers only the paths that reach it. That is the shape that shipped: the write "+
			"sat in the `Initiate` arm and `Carry` returned above it, so the relay's baton hop "+
			"declared nothing and failed as an unreachable peer.", len(late), strings.Join(late, "\n  "))
	}

	if len(bare) > 0 {
		t.Errorf("%d dial site(s) call an initiating verb without declaring a role first "+
			"(ADR-028):\n  %s\n\nThe verb's first frame is a 32-byte verification commitment. "+
			"Sent to an arm expecting a one-byte role, it is refused, the attempt is torn down, "+
			"and the hop surfaces as an unreachable peer — a wire desync reported to the user as "+
			"a dead network.", len(bare), strings.Join(bare, "\n  "))
	}
}
