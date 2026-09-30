package p2p

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"
	"testing"
)

// verifyingCalls are the calls that run a full `sign.Verify` over their first argument —
// ADR-041's pdfcpu read gate plus a hash per signature.
var verifyingCalls = map[string]bool{
	"Verify":               true, // sign.Verify
	"ReadAttestations":     true,
	"ContributionProgress": true,
	"NextContributor":      true,
	"AdmitContribution":    true,
}

// TestNoFunctionVerifiesTheSameDocumentTwice — /pending 711 R5-7. `ContributionProgress` ran
// `sign.Verify(pdf)` and then `ReadAttestations(pdf)`, which is `sign.Verify` again, and
// `coSignExchange` verified the inbound a third time through `AdmitContribution` — three full
// verifies of a peer-chosen document, up to the 128 MiB frame, before the user was asked. Each
// door now verifies once and hands the Status on; this holds that no function in the package
// reaches a verify twice over the SAME expression.
func TestNoFunctionVerifiesTheSameDocumentTwice(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	funcs := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			seen := map[string]int{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				name := ""
				switch f := call.Fun.(type) {
				case *ast.Ident:
					name = f.Name
				case *ast.SelectorExpr:
					name = f.Sel.Name
				}
				if !verifyingCalls[name] {
					return true
				}
				var b strings.Builder
				_ = printer.Fprint(&b, fset, call.Args[0])
				seen[b.String()]++
				return true
			})
			if len(seen) > 0 {
				funcs++
			}
			for arg, n := range seen {
				if n > 1 {
					t.Errorf("%s: %s verifies %q %d times — each is a full read gate and a hash per "+
						"signature over the same bytes; verify once and pass the Status on",
						e.Name(), fn.Name.Name, arg, n)
				}
			}
		}
	}
	// Stimulus: the scan found the functions it is about, so a clean result means something.
	if funcs < 4 {
		t.Fatalf("only %d functions reach a verify — the scan is not reading this package", funcs)
	}
}
