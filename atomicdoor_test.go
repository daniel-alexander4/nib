package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryHandRolledAtomicWriteIsDeclared — the atomic door, policed across the whole repo.
//
// # Why this had to be promoted before P08.S02 wrote a byte
//
// `internal/server/atomicroute_test.go` bans a local atomic-write implementation, and its own doc
// says why: *"What must not exist in this package is a local atomic-write implementation, because
// that is what the twin was and what a future one would be: `os.Rename` of a temp file, sitting
// somewhere nobody compares against the vault's version."*
//
// It scans `os.ReadDir(".")` — **its own package, one of twelve** — and flags the identifier
// `os.Rename` and nothing else. So it is blind from two directions at once, and two live second
// implementations were sitting outside it the whole time: `internal/cli`'s `writeAtomic` and
// `internal/rendezvous`'s node-cache write. P08.S02 puts a durable write in `internal/p2p` or
// `internal/ceremony`, neither of which any guard covers — so the guard is promoted first, on the
// exact model `goroutines_test.go` used when it found `udpmux.readLoop` by the same move.
//
// # What it bans, and what it deliberately does not
//
// The rule is not "never call os". It is that a **temp-file-plus-rename** — the atomic-write
// idiom — must be `internal/atomicfile`'s and not a second copy. So the ban is on `os.Rename`
// reached from anywhere but the door, and every site that legitimately renames or creates
// declares itself in `declared` below with the reason.
//
// A declared exemption is not a hole: it is the same shape `observables_test.go` uses for a
// published field with no reader. What must never happen is a NEW one appearing silently.
func TestEveryHandRolledAtomicWriteIsDeclared(t *testing.T) {
	// declared maps a file to why it renames without being the door. Each entry is a claim
	// somebody made deliberately; a file not in it may not rename at all.
	declared := map[string]string{
		// The door itself. Its whole job is this idiom.
		"internal/atomicfile/atomicfile.go": "the door",
		// **The map held two more until v1.117.261 and now holds none.** `internal/cli/cli.go`'s
		// `writeAtomic` and `internal/rendezvous/dht.go`'s node-cache write were the two
		// second implementations this guard was promoted to find; /pending 316 removed them.
		// Both now call the door. The empty state was the point: an exemption is a claim somebody
		// made deliberately, and the fewer of them there are the more the guard means.
		//
		// **One entry since v1.117.330, and it is a different KIND of rename.** This guard's own
		// message names what it is hunting — *"a temp-file-plus-rename here is a second
		// implementation of the rule internal/atomicfile owns"* — and `CloseOutMirror` is not
		// that. It writes no file and produces no temp: it relocates a whole DIRECTORY, the live
		// ceremony folder to `ended/`, in one syscall. `atomicfile` has no directory door and
		// should not grow one to absorb this; its contract is bytes-to-a-path, and a
		// write-to-temp-then-rename of a directory tree is not a cheaper or safer way to do what
		// `os.Rename` already does atomically.
		//
		// Recorded rather than routed, which is the choice this map exists to make visible.
		"internal/ceremony/closeout.go": "renames a DIRECTORY, not a file — no temp, no bytes",
	}

	fset := token.NewFileSet()
	// Discover the population; never list it — `goroutines_test.go`'s rule, for its reason: a
	// package added later is invisible exactly the way `internal/udpmux` was.
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			// `.claude` holds git WORKTREES a second session creates inside this repo (the
			// rule in CLAUDE.md), each a full copy of the tree. It is gitignored, so it is not
			// source. **Here it fails LOUDLY rather than silently** — the copy's
			// `internal/atomicfile/atomicfile.go` keys under a `.claude/...` path that is absent
			// from `declared`, so the rename branch below reports another session's file as an
			// undeclared door. (The sibling census guard in goroutines_test.go has the opposite
			// failure: its floors are lower bounds, so a doubled tree passes.)
			case ".git", ".claude", "node_modules", "web", "docs", "test":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// STIMULUS for the walk: a walk that parsed nothing gives every check below a clean bill.
	if len(files) < 60 {
		t.Fatalf("setup: only %d source files found — this guard is not walking the repo", len(files))
	}

	renamers, doorUsers, undeclared := 0, 0, 0
	for _, path := range files {
		f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			t.Fatalf("%s: %v", path, perr)
		}
		key := filepath.ToSlash(path)
		usesDoor := false
		ast.Inspect(f, func(n ast.Node) bool {
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
			if pkg.Name == "atomicfile" {
				usesDoor = true
			}
			if pkg.Name == "os" && sel.Sel.Name == "Rename" {
				renamers++
				if _, okDecl := declared[key]; !okDecl {
					undeclared++
					t.Errorf("%s calls os.Rename and is not the atomic door.\n"+
						"A temp-file-plus-rename here is a second implementation of the rule "+
						"internal/atomicfile owns — the one thing atomicroute_test.go's doc says "+
						"must not exist. Route it through atomicfile.Write or WriteDurable, or "+
						"declare it in this guard's `declared` map with the reason.", key)
				}
			}
			return true
		})
		if usesDoor {
			doorUsers++
		}
	}

	// STIMULUS for the check itself, on `atomicroute_test.go`'s model: if nothing in the tree
	// reaches the door, this guard's clean result means the scan is broken, not that the tree is.
	if doorUsers == 0 {
		t.Fatal("no file in the repo calls internal/atomicfile — either the door was renamed or " +
			"this scan is not seeing call expressions, and its clean result above means nothing")
	}
	// **A MOVING FLOOR, and it is the only thing that guards the direction this can actually
	// regress** (/pending 316). Everything above catches a site that renames WITHOUT the door.
	// Nothing above catches a site that quietly stops using the door at all — deleting the write,
	// or swapping it for a bare `os.WriteFile`, leaves this guard perfectly green. So the count is
	// pinned the way `verify_test.go` pins the red-proof count: it may rise, and a fall is a
	// question rather than a pass. Raise it in the same commit as a new door user.
	const doorFloor = 8
	if doorUsers < doorFloor {
		t.Errorf("%d file(s) reach internal/atomicfile and this floor says at least %d. A site "+
			"that stopped calling the door is invisible to every other check here — they all "+
			"look for a rename that bypasses it, and a write that simply became non-atomic has "+
			"no rename to find. If a caller was legitimately removed, lower this figure in the "+
			"same commit and say which one.", doorUsers, doorFloor)
	}
	// And the renamers floor: the declared exemptions really do rename, so a scan that stopped
	// recognising `os.Rename` would be caught here rather than reporting a clean tree.
	if renamers == 0 {
		t.Fatal("no os.Rename found anywhere, including in internal/atomicfile itself — this scan " +
			"is not recognising the call it exists to find")
	}
	t.Logf("atomic door: %d file(s) reach it, %d os.Rename call(s), %d undeclared",
		doorUsers, renamers, undeclared)
}
