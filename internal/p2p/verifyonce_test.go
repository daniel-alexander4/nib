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

// TestEveryVerifyingDoorReachesOneVerify — /pending 785. The census above is per FUNCTION: it sees
// `sign.Verify(pdf)` and `ReadAttestations(pdf)` side by side in one body, which is the shape
// /pending 711 R5-7 removed, and nothing past it — a door that verifies and then hands the BYTES to a
// helper that verifies again is two verifies in two bodies, each counted once.
// `TestWhatTheCeremonyListingCostsPerCeremony` held that half with a clock (`NextContributor` under
// 1.6× a bare verify) and went red under load against good code (1.7× with jsdom alongside, 3/3 green
// alone), because the property is not a time: it is how many verifies a door's code reaches.
//
// So this follows each door's calls through the package's own functions and counts the verifying
// calls reached, each counted once and not descended into (it IS a verify). Every door reaches
// exactly one. Declared limit: a call through a function VALUE or an interface is not followed —
// the package makes none on these paths, and a door that grew one would read here as reaching
// fewer verifies, not more.
func TestEveryVerifyingDoorReachesOneVerify(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]*ast.FuncDecl{} // package-level functions only: a method is not reachable by a bare name
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Body != nil && fn.Recv == nil {
				funcs[fn.Name.Name] = fn
			}
		}
	}
	// reached counts the verifying calls door's body reaches through the package's own functions.
	reached := func(door string) (int, []string) {
		var path []string
		seen := map[string]bool{door: true}
		count := 0
		var walk func(fn *ast.FuncDecl)
		walk = func(fn *ast.FuncDecl) {
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch f := call.Fun.(type) {
				case *ast.SelectorExpr:
					if x, ok := f.X.(*ast.Ident); ok && x.Name == "sign" && f.Sel.Name == "Verify" {
						count++
						path = append(path, fn.Name.Name+" → sign.Verify")
					}
				case *ast.Ident:
					if verifyingCalls[f.Name] {
						count++
						path = append(path, fn.Name.Name+" → "+f.Name)
					} else if callee, ok := funcs[f.Name]; ok && !seen[f.Name] {
						seen[f.Name] = true
						walk(callee)
					}
				}
				return true
			})
		}
		walk(funcs[door])
		return count, path
	}
	doors := 0
	for name := range verifyingCalls {
		if name == "Verify" {
			continue // sign's, not this package's
		}
		if funcs[name] == nil {
			t.Errorf("%s is listed as a verifying door and the package has no such function — the list is stale", name)
			continue
		}
		doors++
		if n, path := reached(name); n != 1 {
			t.Errorf("%s reaches %d verifies, want exactly one — each is ADR-041's full read gate and a hash "+
				"per signature over a document a peer chose; verify once and pass the Status on: %v", name, n, path)
		}
	}
	// Stimulus: the doors this guard exists for are the ones it read.
	if doors < 4 {
		t.Fatalf("only %d verifying doors were followed", doors)
	}
}
