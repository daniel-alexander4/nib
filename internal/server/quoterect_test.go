package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// TestEveryQuoteRouteSizesFromTheOneDoor — ADR-009 over the rule `NominalBlockRect` was written
// for, checked at the DOOR rather than at the values.
//
// **`NominalBlockRect` was created because "the rule had TWO implementations", and it fixed one of
// them.** A hand-copied `{40, 40, 320, 124}` in this package went; a second site stayed, and it was
// the subtler one: `handleCoSignQuote` ran `p2p.NextPlacement(s.docBytes(doc))` — a full PageCount
// plus a sign.Verify over the open document — to publish a rect whose POSITION nothing reads.
// `web/app.js:956` takes `rect[2]-rect[0]` and `rect[3]-rect[1]` and nothing else, and the comment
// beside that call said exactly that while doing the opposite.
//
// The divergence was invisible because the half that differs is the half that is discarded. It
// stopped being invisible the moment placement needed a roster (P07.S06): the responder's block
// goes on the RECEIVED document, so a quote route has no roster and must not acquire one —
// binding to the open document would use the wrong page geometry, which is the reason
// `NominalBlockRect` records for existing.
//
// This asserts ROUTING: every construction of a cosignQuote gets its Rect from NominalBlockRect.
// Comparing the two literals would be the copy agreeing with itself, which is the failure ADR-009
// names and the one that produced this defect.
func TestEveryQuoteRouteSizesFromTheOneDoor(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	var sites, fromDoor int
	var bad []string
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				id, ok := lit.Type.(*ast.Ident)
				if !ok || id.Name != "cosignQuote" {
					return true
				}
				sites++
				for _, el := range lit.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					k, ok := kv.Key.(*ast.Ident)
					if !ok || k.Name != "Rect" {
						continue
					}
					src := exprText(fset, kv.Value)
					if strings.Contains(src, "NominalBlockRect") {
						fromDoor++
					} else {
						bad = append(bad, path+": Rect: "+src)
					}
				}
				return true
			})
		}
	}

	// SETUP: the scan found the quote routes at all. Without this, an empty `bad` is satisfied by
	// a parse that read nothing — the vacuous green this repo keeps finding in its own guards.
	if sites < 2 {
		t.Fatalf("setup: the scan found %d cosignQuote construction(s); there are two routes, so "+
			"it is not reading this package and an empty finding list proves nothing", sites)
	}
	if len(bad) > 0 {
		t.Fatalf("a quote route sizes its block by some other rule than p2p.NominalBlockRect(): "+
			"%v. The client reads only the rect's width and height, so a second rule here is "+
			"invisible until it is wrong (ADR-009).", bad)
	}
	if fromDoor != sites {
		t.Fatalf("%d of %d quote routes set Rect at all — one builds a cosignQuote without it, so "+
			"the client sizes its appearance image to a zero rect", fromDoor, sites)
	}
}

func exprText(fset *token.FileSet, e ast.Expr) string {
	var sb strings.Builder
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			sb.WriteString(id.Name)
			sb.WriteString(".")
		}
		return true
	})
	return sb.String()
}
