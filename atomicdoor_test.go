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
		// **Two second implementations, recorded as findings rather than blessed.** Both predate
		// this guard and both are exactly what `atomicroute_test.go`'s doc describes; they are
		// listed so the guard can go green on today's tree while naming them, and `/pending 316`
		// is where their removal is tracked. Listing them is the honest state — deleting the
		// entries without fixing the code would make this guard red for work nobody scheduled,
		// and leaving them undeclared would make it blind, which is how they survived.
		"internal/cli/cli.go":        "PRE-EXISTING second implementation (/pending 316)",
		"internal/rendezvous/dht.go": "PRE-EXISTING second implementation (/pending 316)",
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
			case ".git", "node_modules", "web", "docs", "test":
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
	// And the renamers floor: the declared exemptions really do rename, so a scan that stopped
	// recognising `os.Rename` would be caught here rather than reporting a clean tree.
	if renamers == 0 {
		t.Fatal("no os.Rename found anywhere, including in internal/atomicfile itself — this scan " +
			"is not recognising the call it exists to find")
	}
	t.Logf("atomic door: %d file(s) reach it, %d os.Rename call(s), %d undeclared",
		doorUsers, renamers, undeclared)
}
