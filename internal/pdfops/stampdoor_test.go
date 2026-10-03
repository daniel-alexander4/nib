package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryPdfcpuStampGoesThroughStampInPlace — stampInPlace is the one door a pdfcpu stamp runs through
// (ADR-009, /pending 457): pdfcpu turns a turned page about the origin to stamp it, and only the door turns it
// back about its box's corner. A `pdfcpu.AddWatermarks*` call must sit inside a function literal handed to
// stampInPlace, or to stampTextWatermarks, which hands its own to stampInPlace; and no code may stamp through
// pdfcpu's `api` package, whose read-stamp-write the door cannot reach inside.
func TestEveryPdfcpuStampGoesThroughStampInPlace(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	stamps, routed := 0, 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		// The spans of every function literal handed to a door.
		var inside [][2]token.Pos
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok && (id.Name == "stampInPlace" || id.Name == "stampTextWatermarks") {
				for _, a := range call.Args {
					if lit, ok := a.(*ast.FuncLit); ok {
						inside = append(inside, [2]token.Pos{lit.Pos(), lit.End()})
					}
				}
				if id.Name == "stampInPlace" {
					routed++
				}
			}
			return true
		})
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !strings.HasPrefix(sel.Sel.Name, "AddWatermarks") {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "pdfcpu" {
				return true
			}
			stamps++
			for _, span := range inside {
				if call.Pos() >= span[0] && call.End() <= span[1] {
					return true
				}
			}
			t.Errorf("%s stamps through pdfcpu outside stampInPlace", fset.Position(call.Pos()))
			return true
		})
	}
	// stampTextWatermarks is a door only because it routes its own stamp through stampInPlace.
	src, err := os.ReadFile("stampwrite.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "stampInPlace(ctx, func() error { return add(ctx, emb) })") {
		t.Errorf("stampTextWatermarks no longer runs its stamp through stampInPlace")
	}
	// No stamp through pdfcpu's api package, anywhere nib's code lives.
	apiStamp := regexp.MustCompile(`\bapi\.(AddWatermarks|AddStamps|AddTextWatermarks|AddImageWatermarks|AddPDFWatermarks|AddTextStamps|AddImageStamps|AddPDFStamps)\w*\(`)
	for _, root := range []string{"..", "../../cmd"} {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Error(rerr)
				return nil
			}
			if loc := apiStamp.FindIndex(b); loc != nil {
				t.Errorf("%s stamps through pdfcpu's api package (%q), past stampInPlace", path, b[loc[0]:loc[1]])
			}
			return nil
		})
	}
	// The stimulus: the census saw the stamps it exists for.
	if stamps < 4 || routed < 2 {
		t.Fatalf("census saw %d pdfcpu stamps and %d stampInPlace calls; it is not reading the package", stamps, routed)
	}
}
