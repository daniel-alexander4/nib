package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestEveryCeremonyThisServerUsesIsGated — the guard that makes `ensureBootstrapped`'s
// nil-means-allowed safe (`/pending 451`).
//
// # What it closes
//
// The rendezvous switch is read through `ceremonyID.rzOn`, a predicate the server stamps on with
// `gateRendezvous`. `ensureBootstrapped` treats a nil predicate as ALLOWED, which it has to: the
// CLI builds ceremonies with no server behind them, and so do a dozen tests. That leaves exactly
// one hole — a server path that builds a `ceremonyID` and forgets the stamp reaches the public DHT
// with the switch off, silently, and every other check in the file passes.
//
// # How
//
// Parsed, not grepped. Every production file in this package is walked for a call to
// `ceremonyFor(` or a `&ceremonyID{` literal, and each one must be stamped: either the call is
// itself an argument to `gateRendezvous`, or the enclosing function assigns through it within a few
// lines. A text scan cannot see the second shape, and this repo has twice been bitten by a
// comment satisfying a word-match (`/pending 410`, `/pending 445`).
func TestEveryCeremonyThisServerUsesIsGated(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	scanned, found := 0, 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, rerr := os.ReadFile(name)
		if rerr != nil {
			t.Fatal(rerr)
		}
		f, perr := parser.ParseFile(fset, name, src, 0)
		if perr != nil {
			t.Fatalf("%s: %v", name, perr)
		}
		scanned++
		lines := strings.Split(string(src), "\n")
		ast.Inspect(f, func(n ast.Node) bool {
			var pos token.Pos
			switch x := n.(type) {
			case *ast.CallExpr:
				id, ok := x.Fun.(*ast.Ident)
				if !ok || id.Name != "ceremonyFor" {
					return true
				}
				pos = x.Pos()
			case *ast.CompositeLit:
				// `&ceremonyID{…}` — the literal arrives as the CompositeLit inside a UnaryExpr.
				id, ok := x.Type.(*ast.Ident)
				if !ok || id.Name != "ceremonyID" {
					return true
				}
				pos = x.Pos()
			default:
				return true
			}
			// **One exemption, and it is the constructor itself.** `ceremonyFor` is package-level
			// with no server behind it — it is what the CLI and the tests call — so it cannot
			// stamp, and requiring it to would mean plumbing a predicate through every caller to
			// reach the one place that already has one. Its CALLERS are what this guard polices,
			// and they are all in this package.
			if enclosing(f, fset, pos) == "ceremonyFor" {
				return true
			}
			found++
			// The window is the construction's own line plus the two after it, which covers both
			// shapes in the tree: `return s.gateRendezvous(&ceremonyID{…})` on one line, and
			// `cer, err := ceremonyFor(…)` followed by `cer = s.gateRendezvous(cer)`.
			ln := fset.Position(pos).Line
			window := strings.Join(lines[max(0, ln-1):min(len(lines), ln+2)], "\n")
			if !strings.Contains(window, "gateRendezvous") {
				t.Errorf("%s:%d builds a ceremonyID and does not pass it through gateRendezvous. "+
					"`ensureBootstrapped` reads the rendezvous switch from `rzOn`, and an unstamped "+
					"ceremony has none — so this path reaches the public DHT with the feature "+
					"switched off, and nothing else in the package would notice", name, ln)
			}
			return true
		})
	}
	// Floors, because a scan that found nothing passes silently — the shape this repo has paid for
	// more than once. Three production constructions exist today.
	if scanned < 30 {
		t.Errorf("only %d production files scanned; this package is much larger, so the walk is "+
			"not reaching them", scanned)
	}
	if found < 3 {
		t.Errorf("only %d ceremonyID construction(s) found, want at least 3 — the matcher has "+
			"stopped recognising them and every assertion above is vacuous", found)
	}
}

// enclosing names the function a position sits inside, or "" at file scope.
func enclosing(f *ast.File, fset *token.FileSet, pos token.Pos) string {
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Pos() <= pos && pos <= fn.End() {
			return fn.Name.Name
		}
	}
	return ""
}
