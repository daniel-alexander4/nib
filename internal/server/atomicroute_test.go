package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryFileWriteGoesThroughTheAtomicDoor — /pending 287's second half, asserted on the ROUTING.
//
// # What this is about
//
// `internal/server` had its own `writeFileAtomic`: rename-only, no fsync, four callers. Beside it
// `internal/vault` had a function of the SAME NAME with a different contract — temp file, fsync,
// rename, parent-dir fsync — and `handleVaultImport` reached for the wrong one to replace
// `vault.nib`. `vault.go` records what that cost: a power loss inside the writeback window leaves
// the vault present and garbage while the original, the only copy of the identity, is already gone.
//
// `internal/atomicfile` was built as the one door and `internal/vault` routes through it. **Nothing
// asserted that `internal/server` did**, which is the ADR-009 shape exactly: a rule with a door and
// a package that does not use it.
//
// # Why it is structural
//
// Durability has no cheap observation. Proving a write survived a crash needs a crash; proving one
// did NOT is proving a negative about the page cache. What is checkable — and what actually
// regressed here — is whether a caller reaches the door at all.
//
// # Why it bans the NAME rather than checking the calls
//
// A test that only required `atomicfile.` to appear would pass a file that called the door once and
// hand-rolled a second write below it. What must not exist in this package is a local atomic-write
// implementation, because that is what the twin was and what a future one would be: `os.Rename` of
// a temp file, sitting somewhere nobody compares against the vault's version.
func TestEveryFileWriteGoesThroughTheAtomicDoor(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	renamers, doorUsers := 0, 0
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		raw, rerr := os.ReadFile(e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		src := string(raw)
		fset := token.NewFileSet()
		af, perr := parser.ParseFile(fset, e.Name(), src, parser.ParseComments)
		if perr != nil {
			t.Fatal(perr)
		}
		// Comments stripped: a scan satisfied by prose that merely names the call is how
		// `handleSave`'s freeze guard read its own explanation as proof of coverage (v1.117.155).
		code := stripLineComments(src)
		if strings.Contains(code, "atomicfile.") {
			doorUsers++
		}
		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Rename" {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Name != "os" {
				return true
			}
			renamers++
			t.Errorf("%s calls os.Rename directly. A temp-file-plus-rename written here is a "+
				"second atomic-write implementation, which is exactly what internal/vault's "+
				"same-named twin was — and reaching for the wrong one replaced vault.nib without "+
				"durability. Call internal/atomicfile and choose Write or WriteDurable by whether "+
				"this caller holds the only copy.", filepath.Base(e.Name()))
			return true
		})
	}
	// **The stimulus, and it is not decoration.** A scan that found no `os.Rename` because it read
	// the wrong directory, or because a parse silently failed, looks identical to a clean pass.
	// This package really does write files, so some file really must reach the door.
	if doorUsers == 0 {
		t.Fatal("no file in internal/server mentions internal/atomicfile at all — this scan is " +
			"either reading the wrong directory or the package has stopped writing files, and " +
			"either way its clean result above means nothing")
	}
	t.Logf("files routing through internal/atomicfile: %d; direct os.Rename calls: %d", doorUsers, renamers)
}
