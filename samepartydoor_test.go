package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryFingerprintComparisonIsTheSamePartyDoor — /pending 636, ADR-009 over ADR-051's rule.
//
// # The rule
//
// "An empty fingerprint matches nothing" (ADR-051, ADR-058): "" is what nib reports for a signer
// it could not identify, and `strings.EqualFold("", "")` is true. The rule is written once, in
// `p2p.SameParty`, and a site that compares a fingerprint read from a SIGNATURE calls it.
//
// # What is checked — the routing, not the text each site prints
//
//   - `internal/p2p` and `internal/cli`, every non-test file: no `strings.EqualFold`, no
//     `strings.ToLower` and no `==` / `!=` (against anything but a literal) over a `.Fingerprint`
//     or `.AcceptedPeer` selector, outside `SameParty` itself. No exemptions: these are the
//     packages that build and judge `SignerAttestation`s.
//   - `internal/server`: the same, inside any function that reads `.Signers` or calls
//     `Attestations` / `ReadAttestations` — the functions that hold a signature's fingerprint.
//
// # Declared blind spots
//
// Syntax, not types. A signature's fingerprint copied into a local and compared under another name
// is not seen, and neither is a server function handed attestations as a parameter under a name of
// its own. `internal/server`'s roster-against-roster and roster-against-this-machine comparisons
// are outside it on purpose: neither side is read from a signature, and a roster fingerprint is
// refused unless it is 32 bytes (`ceremony.RosterHash`).
//
// # Anti-vacuity
//
// A floor on the door's callers per package: a matcher that stops matching reports full coverage.
func TestEveryFingerprintComparisonIsTheSamePartyDoor(t *testing.T) {
	fset := token.NewFileSet()
	doorCalls := map[string]int{}
	var bypasses []string

	isFP := func(e ast.Expr) bool {
		sel, ok := e.(*ast.SelectorExpr)
		return ok && (sel.Sel.Name == "Fingerprint" || sel.Sel.Name == "AcceptedPeer")
	}
	isLiteral := func(e ast.Expr) bool {
		_, ok := e.(*ast.BasicLit)
		return ok
	}
	// scan reports every copy of the comparison under n, and counts the door's calls.
	scan := func(pkg, file string, n ast.Node, count bool) {
		ast.Inspect(n, func(n ast.Node) bool {
			at := func(what string) {
				bypasses = append(bypasses, file+":"+
					strings.TrimPrefix(fset.Position(n.Pos()).String(), fset.Position(n.Pos()).Filename+":")+" "+what)
			}
			switch x := n.(type) {
			case *ast.CallExpr:
				switch calleeName(x.Fun) {
				case "SameParty":
					if count {
						doorCalls[pkg]++
					}
				case "EqualFold", "ToLower":
					for _, a := range x.Args {
						if isFP(a) {
							at("folds a fingerprint with strings." + calleeName(x.Fun))
						}
					}
				}
			case *ast.BinaryExpr:
				if x.Op != token.EQL && x.Op != token.NEQ {
					return true
				}
				if (isFP(x.X) && !isLiteral(x.Y)) || (isFP(x.Y) && !isLiteral(x.X)) {
					at("compares a fingerprint with " + x.Op.String())
				}
			}
			return true
		})
	}
	// holdsSigners reports a function that reads a signature's fingerprint at its source.
	holdsSigners := func(fn *ast.FuncDecl) bool {
		found := false
		ast.Inspect(fn, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if x.Sel.Name == "Signers" {
					found = true
				}
			case *ast.CallExpr:
				if c := calleeName(x.Fun); c == "Attestations" || c == "ReadAttestations" {
					found = true
				}
			}
			return !found
		})
		return found
	}

	scanned := 0
	for _, pkg := range []string{"p2p", "cli", "server"} {
		files, err := filepath.Glob(filepath.Join("internal", pkg, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range files {
			if strings.HasSuffix(p, "_test.go") {
				continue
			}
			src, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parser.ParseFile(fset, p, src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			scanned++
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok {
					scan(pkg, filepath.ToSlash(p), d, pkg != "server")
					continue
				}
				if pkg == "p2p" && fn.Name.Name == "SameParty" && fn.Recv == nil {
					continue // the door itself
				}
				if pkg == "server" && !holdsSigners(fn) {
					// Still counted, so the floor sees a door call wherever it sits.
					ast.Inspect(fn, func(n ast.Node) bool {
						if c, ok := n.(*ast.CallExpr); ok && calleeName(c.Fun) == "SameParty" {
							doorCalls[pkg]++
						}
						return true
					})
					continue
				}
				scan(pkg, filepath.ToSlash(p), fn, true)
			}
		}
	}
	if scanned < 100 {
		t.Fatalf("the scan read %d non-test file(s) across internal/p2p, internal/cli and internal/server — "+
			"too few to be those packages", scanned)
	}
	// The floors are the counts when the door was built (2026-10-10): 16, 1 and 1.
	for pkg, floor := range map[string]int{"p2p": 16, "cli": 1, "server": 1} {
		if doorCalls[pkg] < floor {
			t.Errorf("internal/%s calls SameParty %d time(s), fewer than the %d it did when the door was built — "+
				"a site has gone back to comparing fingerprints itself, or this scan stopped matching",
				pkg, doorCalls[pkg], floor)
		}
	}
	sort.Strings(bypasses)
	for _, b := range bypasses {
		t.Errorf("%s — route it through p2p.SameParty: an empty fingerprint matches nothing (ADR-051), and a "+
			"comparison written at the site is a second copy of that rule", b)
	}
}
