package server

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestNoTestWritesIntoTheDevelopersRealHome — the guard for a defect that shipped 532 ceremonies
// into a real `~/nib`.
//
// # What happened
//
// `defaultOutputDir()` is `$HOME/nib`, read at call time. The fixture `ceremonyOnDisk` ends in
// `ceremony.WriteMirror(defaultOutputDir(), …)`, and sixteen of its seventeen callers sandboxed
// `HOME` first. The seventeenth called it on its first line. One ceremony per run, every run, for
// days — and the product then LISTED them, so the Signing Ceremonies panel filled with identical
// "We agree" cards for proceedings nobody had convened. Found by Dan, from a screenshot.
//
// # Why no tier could see it
//
// It is invisible by construction. The test passes; the suite is green; the damage is outside the
// repository, in a directory no harness looks at. `go test ./...` cannot fail for writing to the
// machine it runs on, and tiers 2, 3, 4 and 6 all use their own throwaway homes — so the one thing
// that noticed was a person opening the app.
//
// # The rule
//
// A test that reaches a writer under `defaultOutputDir()` must first put `HOME` somewhere
// disposable. Three helpers already do it — `startServerWith`, `openTestServer`, `startServer` —
// and a literal `t.Setenv("HOME", t.TempDir())` is the fourth way. The scan asks only that ONE of
// them appears earlier in the same function, which is what makes it cheap and unambiguous.
//
// **Reads, not just writes.** `ReadStored(defaultOutputDir(), …)` against a real home does not
// corrupt anything, but it makes the test's result depend on the developer's own ceremonies — a
// pass or a fail that no one else can reproduce. Both go in the population.
func TestNoTestWritesIntoTheDevelopersRealHome(t *testing.T) {
	// **Parsed, never matched, and the first cut of this scan flagged ITSELF.** A text scan reads
	// the needles out of its own explanatory comments and out of the error message below, so it
	// reported two offenders that were prose. `ast.Inspect` visits neither comments nor the inside
	// of string literals, and it carries positions — which is what an ORDER rule needs.
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	// Callees that reach the real home, and callees that move it somewhere disposable first.
	//
	// **`ceremonyOnDisk` is PROSPECTIVE and it is not probed — said plainly, because the rest of
	// this file's clauses are.** A fixture reaches the home on its caller's behalf, and a caller
	// cannot tell by reading its own line, which is exactly how the defect got in. But every
	// caller today ALSO calls `defaultOutputDir()` directly, so dropping this entry does not go
	// red on its own: the first name catches them anyway. It earns its place the day somebody
	// writes a test whose only route to the home is the fixture — which is the shape that cost
	// 532 ceremonies.
	reaches := map[string]bool{"defaultOutputDir": true, "ceremonyOnDisk": true}
	sandbox := map[string]bool{"startServerWith": true, "openTestServer": true, "startServer": true}

	fset := token.NewFileSet()
	var offenders []string
	scanned, reached := 0, 0

	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, e.Name(), nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", e.Name(), perr)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || !strings.HasPrefix(fd.Name.Name, "Test") {
				continue
			}
			scanned++
			firstReach, firstSandbox := token.NoPos, token.NoPos
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					if reaches[fn.Name] && (!firstReach.IsValid() || call.Pos() < firstReach) {
						firstReach = call.Pos()
					}
					if sandbox[fn.Name] && (!firstSandbox.IsValid() || call.Pos() < firstSandbox) {
						firstSandbox = call.Pos()
					}
				case *ast.SelectorExpr:
					// `t.Setenv("HOME", …)` — the literal is read from the AST, so a comment
					// mentioning HOME cannot stand in for the call.
					if fn.Sel.Name != "Setenv" || len(call.Args) == 0 {
						return true
					}
					lit, ok := call.Args[0].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING || strings.Trim(lit.Value, `"`) != "HOME" {
						return true
					}
					if !firstSandbox.IsValid() || call.Pos() < firstSandbox {
						firstSandbox = call.Pos()
					}
				}
				return true
			})
			if !firstReach.IsValid() {
				continue
			}
			reached++
			if !firstSandbox.IsValid() || firstSandbox > firstReach {
				offenders = append(offenders,
					fmt.Sprintf("%s (%s)", fd.Name.Name, filepath.Base(e.Name())))
			}
		}
	}
	// Two stimulus floors. A walk that read no files, or one that found nothing reaching the real
	// home, reports zero offenders — which is byte-identical to a clean run and is how a scan goes
	// quietly blind.
	if scanned < 100 {
		t.Fatalf("the walk read %d test functions in this package, which has hundreds — it is not "+
			"reading the directory and an empty report means nothing", scanned)
	}
	if reached < 10 {
		t.Fatalf("only %d test function(s) reach defaultOutputDir() or a fixture that does. This "+
			"package has many; the population is wrong, so a clean report is vacuous", reached)
	}

	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Errorf("%d test(s) reach the real $HOME without sandboxing it first. `defaultOutputDir()` "+
			"is `$HOME/nib`, so these write ceremonies into the developer's own Nib — where the "+
			"product then LISTS them. 532 accumulated that way before anyone noticed, and no tier "+
			"can see it: the damage is outside the repository. Put `t.Setenv(\"HOME\", t.TempDir())` "+
			"first, or use startServerWith/openTestServer/startServer, which already do.\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
