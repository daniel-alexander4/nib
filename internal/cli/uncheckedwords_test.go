package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"

	"nib/internal/sign"
)

// TestEveryUncheckedCauseHasACLISentence is `internal/sign`'s TestEveryUncheckedCauseIsSaid for the
// CLI (/pending 741): every `sign.UncheckedCause` constant, read from the source so a new one is
// counted without anyone listing it, has a sentence in `uncheckedWords`, and no sentence is for a cause
// nothing returns.
func TestEveryUncheckedCauseHasACLISentence(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "sign", "verify.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	causes := map[sign.UncheckedCause]bool{}
	for _, d := range file.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, sp := range g.Specs {
			vs := sp.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "UncheckedCause" {
				continue
			}
			for _, v := range vs.Values {
				if lit, ok := v.(*ast.BasicLit); ok {
					c, _ := strconv.Unquote(lit.Value)
					causes[sign.UncheckedCause(c)] = true
				}
			}
		}
	}
	if len(causes) < 3 {
		t.Fatalf("setup: %d causes read", len(causes))
	}
	for c := range causes {
		if uncheckedWords[c] == "" {
			t.Errorf("the unchecked cause %q has no CLI sentence", c)
		}
	}
	for c := range uncheckedWords {
		if !causes[c] {
			t.Errorf("uncheckedWords has a sentence for %q, which is no UncheckedCause", c)
		}
	}
}
