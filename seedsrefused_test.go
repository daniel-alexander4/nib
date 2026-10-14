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

// TestInvitationSeedsReachNothingButTheDiagnostic — ADR-152, /pending 357.
//
// Invitation-carried DHT seeds are refused on both halves: no door that issues an invitation sets
// `Seeds`, and nothing hands a parsed invitation's `Seeds` to `rendezvous.Server.Seed`. Until this
// guard the refusal was a comment at the convene door, and either half could be wired by a change
// that never read it — the consuming half is the one that arms the eclipse chain.
//
// The one caller is `internal/cli/rendezvous.go`, the `nib rendezvous` diagnostic, which samples
// its own table and feeds itself. `internal/rendezvous` and `internal/ceremony` define and
// validate the things named here and are not callers.
func TestInvitationSeedsReachNothingButTheDiagnostic(t *testing.T) {
	const diagnostic = "internal/cli/rendezvous.go"
	fset := token.NewFileSet()
	var offenders []string
	var diagnosticUses int
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(path)
		if d.IsDir() {
			switch {
			case rel == "third_party", rel == "node_modules", strings.HasPrefix(d.Name(), ".") && rel != ".",
				rel == "internal/rendezvous", rel == "internal/ceremony":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			var what string
			switch v := n.(type) {
			case *ast.SelectorExpr:
				x, _ := v.X.(*ast.Ident)
				switch {
				case v.Sel.Name == "SeedSample":
					what = "SeedSample"
				case v.Sel.Name == "Seeds":
					what = ".Seeds"
				// `addrscope.Seed` is the address rule and `maphash.Seed` a hash key (the mux's
				// connection-id tag), different things with the same name.
				case v.Sel.Name == "Seed" && (x == nil || (x.Name != "addrscope" && x.Name != "maphash")):
					what = ".Seed("
				}
			case *ast.KeyValueExpr:
				if k, ok := v.Key.(*ast.Ident); ok && k.Name == "Seeds" {
					what = "Seeds:"
				}
			}
			if what == "" {
				return true
			}
			if rel == diagnostic {
				diagnosticUses++
				return true
			}
			offenders = append(offenders, fset.Position(n.Pos()).String()+" "+what)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if diagnosticUses < 3 {
		t.Fatalf("setup: the scan found %d use(s) in %s, and it samples, sets and feeds seeds — "+
			"the scan is not reading the tree", diagnosticUses, diagnostic)
	}
	for _, o := range offenders {
		t.Errorf("%s — invitation seeds are refused outside the `nib rendezvous` diagnostic "+
			"(docs/adr/152): setting them on an issued invitation is inert, and feeding a parsed "+
			"invitation's to the DHT lets its sender choose where this machine's first packets go", o)
	}
}
