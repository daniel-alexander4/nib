package vault

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

// TestEveryVaultMutationGoesThroughOneDoor — ADR-009, `/pending 510`.
//
// # The rule
//
// A mutator that changes the vault in memory and then fails to persist leaves memory AHEAD of
// disk: the running Nib behaves as though the change was stored and the next launch disagrees.
// Nineteen mutators assigned and then saved, over twenty-two call sites; four of them hand-rolled
// a restore and fifteen did not. ADR-009: the rule is written once — `mutateLocked` — and every
// site calls it.
//
// # What this asserts, and why it is the ROUTING and not the restores
//
// Two things, and each one alone is escapable:
//
//   - **No function outside the door may change `v.contents` or `v.ssh`.** An assignment whose
//     left-hand side is rooted at either field — including through an index, a slice, or a
//     pointer taken with `&` — must sit inside the closure handed to `mutateLocked`, where the
//     snapshot has already been taken. In front of the call is the defect with a door beside it.
//   - **No function outside the door may call `save()`**, except the exported `Save`, which
//     persists without mutating and so has nothing to put back.
//
// Comparing the restores for agreement would have said nothing about a twenty-third site added
// without one, which is the failure mode ADR-009 names. This goes red when a new mutator assigns
// before the door, which is the shape that ships.
//
// **Declared blind spot: mutation through a value the AST cannot follow home.** A helper handed
// `v.contents.PinnedPeers` as an argument and writing into it, or a closure stored in a variable
// and called later, is not seen: this reads syntax, not aliasing. The `&` case is covered because
// it is the one that appears here — `AddCeremonySecret` upserts through
// `&v.contents.CeremonySecrets[i]` — and because taking the address is how a mutation leaves the
// syntax the rest of this scan can see.
func TestEveryVaultMutationGoesThroughOneDoor(t *testing.T) {
	const door = "mutateLocked"
	// Both rows are deliberate exemptions and each carries why, per ADR-009.
	exempt := map[string]string{
		// The payload version is stamped on every write at the one place they all pass
		// through — save() says so at the line — and it is inside the door's own save.
		"save": "stamps Contents.Version inside the write the door performs",
		// The door itself: it takes the snapshot and puts it back.
		door: "is the door",
	}
	// Save() persists without mutating, so there is nothing for it to roll back.
	exemptSaveCallers := map[string]string{
		"Save": "persists what is already in memory; it mutates nothing",
		door:   "is the door",
	}

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	var offenders []string
	doorDeclared := ""
	doorCalls := 0
	mutations := 0
	files := 0

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		// Mode 0 attaches no comments, and ast.Inspect never enters a string literal, so
		// neither a comment nor a string can launder a site in or out of this scan.
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", name, perr)
		}
		files++

		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			if fd.Name.Name == door && fd.Recv != nil {
				doorDeclared = fmt.Sprintf("%s:%d", name, fset.Position(fd.Pos()).Line)
			}

			// The closures the door snapshots around. Collected first, because an assignment
			// is judged by whether it sits inside one.
			var inside [][2]token.Pos
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || calleeName(call.Fun) != door {
					return true
				}
				doorCalls++
				for _, arg := range call.Args {
					if lit, ok := arg.(*ast.FuncLit); ok {
						inside = append(inside, [2]token.Pos{lit.Pos(), lit.End()})
					}
				}
				return true
			})
			covered := func(p token.Pos) bool {
				for _, r := range inside {
					if p >= r[0] && p < r[1] {
						return true
					}
				}
				return false
			}

			flag := func(what string, p token.Pos, field string) {
				mutations++
				if _, ok := exempt[fd.Name.Name]; ok {
					return
				}
				if covered(p) {
					return
				}
				offenders = append(offenders, fmt.Sprintf("%s:%d %s changes v.%s (%s)",
					name, fset.Position(p).Line, fd.Name.Name, field, what))
			}

			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range x.Lhs {
						if field := rootedAtVaultState(lhs); field != "" {
							flag("assigned before the door", lhs.Pos(), field)
						}
					}
				case *ast.IncDecStmt:
					if field := rootedAtVaultState(x.X); field != "" {
						flag("incremented before the door", x.Pos(), field)
					}
				case *ast.UnaryExpr:
					// `s := &v.contents.CeremonySecrets[i]` — the write that follows is
					// through s, where no scan rooted at v.contents can see it.
					if x.Op == token.AND {
						if field := rootedAtVaultState(x.X); field != "" {
							flag("has its address taken before the door", x.Pos(), field)
						}
					}
				case *ast.CallExpr:
					if calleeName(x.Fun) != "save" {
						return true
					}
					if _, ok := exemptSaveCallers[fd.Name.Name]; ok {
						return true
					}
					offenders = append(offenders, fmt.Sprintf("%s:%d %s calls save() itself",
						name, fset.Position(x.Pos()).Line, fd.Name.Name))
				}
				return true
			})
		}
	}

	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("a vault change outside the door: %s\n\t"+
			"Wrap the change in v.%s(func(){ … }). It snapshots what save() persists, applies, "+
			"writes, and puts memory back when the write fails — in front of the call, a failed "+
			"save leaves this Nib behaving as though the change was stored and the next launch "+
			"disagreeing (/pending 510).", o, door)
	}

	// ── The floor ────────────────────────────────────────────────────────────────────────────
	//
	// An empty offender list is also what a scan that had stopped matching anything reports, and
	// this one matches on identifiers that a rename would carry away silently. Three counts, and
	// each fails for a different reason: the door gone, the sites gone, the scan gone.
	if files == 0 {
		t.Fatal("the scan read no source files — it is not reading the package")
	}
	if doorDeclared == "" {
		t.Fatalf("no method %q is declared in this package — it was renamed or removed, and "+
			"every assertion above has been matching nothing", door)
	}
	if doorCalls < 15 {
		t.Errorf("v.%s has %d call sites; the sites that used to assign and then save are "+
			"twenty-two. Fewer means a site stopped routing through the door — or that this "+
			"scan no longer finds calls at all", door, doorCalls)
	}
	// Assignments SEEN, exempt and covered ones included: it proves the left-hand-side matcher
	// still recognises the shapes in this file, which the offender list cannot, because the
	// offender list is empty in exactly the two cases that matter.
	if mutations < 20 {
		t.Errorf("the scan recognised %d change(s) to v.contents/v.ssh in this package; there "+
			"are more than twenty. rootedAtVaultState has stopped matching the shapes it is "+
			"written for, and a clean run above means nothing", mutations)
	}
}

// rootedAtVaultState reports which piece of persisted vault state an expression writes to —
// "contents", "ssh", or "" for anything else — by walking down through selectors, indexes and
// slices to whatever the expression is ultimately rooted at.
func rootedAtVaultState(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && id.Name == "v" {
				if x.Sel.Name == "contents" || x.Sel.Name == "ssh" {
					return x.Sel.Name
				}
				return ""
			}
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.SliceExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return ""
		}
	}
}

// calleeName is the function name of a call, bare or through a receiver.
func calleeName(fun ast.Expr) string {
	switch e := fun.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}
