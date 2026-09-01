package p2p

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// /pending 336 — the re-delivery cache must be consulted BEFORE the consent gate, and
// until now that was written down in two prose comments and asserted nowhere.
//
// `checkArrival` reserves a literal `0` deadline budget, and its comment
// (`internal/server/ceremonyid.go`) justifies the placement in so many words: *"this runs
// downstream of `rd.Cached`, so a party who already signed can still hand that signature
// over after the deadline. A gate above the cache would refuse a re-delivery and destroy
// a signature already made — the outcome D24 exists to prevent."*
//
// That justification is a **call-graph fact about a different package**: inside
// `coSignExchange`, `rd.Cached` returns before `c.Confirm` is reached. Re-order those two
// and the zero budget stops being safe — a re-delivery arriving after the deadline would
// be refused at the gate, and a signature the party already made would be destroyed. The
// suite would stay green, because the two structural guards that exist
// (`ceremonypin_test.go`) pin what is inside `checkArrival` and what precedes it inside
// `Confirm`; neither reads `coSignExchange` at all.
//
// **Parsed, not grepped, and that is not fastidiousness.** `session.go` mentions
// `rd.Cached` in a COMMENT hundreds of lines above the real call — a text scan finds the
// comment first and would report the correct order for any arrangement of the actual
// code, which is the vacuous green this file exists to refuse. The AST sees calls only.
func TestTheReDeliveryCacheIsConsultedBeforeTheConsentGate(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "session.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing session.go: %v", err)
	}

	var fn *ast.FuncDecl
	for _, d := range file.Decls {
		if f, ok := d.(*ast.FuncDecl); ok && f.Name.Name == "coSignExchange" {
			fn = f
			break
		}
	}
	if fn == nil {
		t.Fatal("coSignExchange is not in session.go — this guard is reading nothing, and the ordering it protects is unasserted again")
	}

	// Position of the first call to each, in source order within the function.
	pos := func(recv, sel string) token.Pos {
		var found token.Pos
		ast.Inspect(fn, func(n ast.Node) bool {
			if found != token.NoPos {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			s, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || s.Sel.Name != sel {
				return true
			}
			id, ok := s.X.(*ast.Ident)
			if !ok || id.Name != recv {
				return true
			}
			found = call.Pos()
			return false
		})
		return found
	}

	cached := pos("rd", "Cached")
	confirm := pos("c", "Confirm")

	// Both must be FOUND, or the comparison below is satisfied by an absence — the shape
	// that makes an ordering guard pass against a function that no longer calls either.
	if cached == token.NoPos {
		t.Fatal("coSignExchange contains no call to rd.Cached — the re-delivery cache is not consulted here at all, so a party who already signed cannot hand that signature over and D24's protection is gone")
	}
	if confirm == token.NoPos {
		t.Fatal("coSignExchange contains no call to c.Confirm — the consent gate is not reached, so this function signs without asking")
	}
	if !(cached < confirm) {
		t.Errorf("rd.Cached is called at %s and c.Confirm at %s — the cache must be consulted FIRST. checkArrival reserves a ZERO deadline budget specifically because it runs downstream of the cache; with this order reversed, a re-delivery arriving after the ceremony deadline is refused at the gate and a signature the party already made is destroyed",
			fset.Position(cached), fset.Position(confirm))
	}
}
