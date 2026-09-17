package nib

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryMissingToolRefusalGoesThroughOneDoor — ADR-040.
//
// # The rule
//
// "An optional converter could not be found" was classified at FOUR sites, each with its own
// `errors.Is` branch and its own literal: two HTTP handlers and two CLI verbs, for two tools.
// Six strings, three different wordings per tool, and the sentinel's own text — which no user
// ever saw, because every site substituted — made two more. ADR-009: a rule holding at more
// than one call site is written once and every site calls it.
//
// # What this asserts, and what it deliberately does not
//
// It asserts ROUTING: no file outside `internal/pdfops` may name the sentinels, because naming
// one is how a site classifies for itself. It does NOT compare the sentences those sites print
// — `internal/server/handoff.go` states the repo's reading where `readInstallablePDF` is
// consumed: *"ADR-009 unifies the CHECKS; it explicitly does not require every site to print
// the same sentence."* A CLI user who typed `--gs` needs to hear about `--gs`; a GUI user needs
// a link. Eight copies checked for agreement would say nothing about a ninth site added
// without one, which is the failure mode ADR-009 names.
func TestEveryMissingToolRefusalGoesThroughOneDoor(t *testing.T) {
	const doorPkg = "internal/pdfops"
	sentinels := map[string]bool{
		"ErrLibreOfficeMissing": true,
		"ErrGhostscriptMissing": true,
	}

	repo, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()

	var offenders []string
	doorCalls := 0
	doorCallers := map[string]bool{}
	declaredIn := map[string]string{}
	files := 0

	walkErr := filepath.WalkDir(repo, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "web", "test", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, p, nil, 0)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", p, perr)
		}
		files++
		rel, _ := filepath.Rel(repo, p)
		inDoor := strings.HasPrefix(rel, doorPkg+string(filepath.Separator))

		ast.Inspect(f, func(n ast.Node) bool {
			// Where the sentinels are DECLARED — the floor reads this.
			if vs, ok := n.(*ast.ValueSpec); ok && inDoor {
				for _, name := range vs.Names {
					if sentinels[name.Name] {
						declaredIn[name.Name] = fmt.Sprintf("%s:%d", rel, fset.Position(name.Pos()).Line)
					}
				}
			}
			// Any mention of a sentinel outside the door is a site classifying for itself.
			// Parsing with mode 0 attaches no comments and ast.Inspect never enters a string
			// literal, so neither a comment nor a string can launder a site in or out.
			if sel, ok := n.(*ast.SelectorExpr); ok && !inDoor && sentinels[sel.Sel.Name] {
				offenders = append(offenders, fmt.Sprintf("%s:%d names %s",
					rel, fset.Position(sel.Pos()).Line, sel.Sel.Name))
			}
			if id, ok := n.(*ast.Ident); ok && inDoor && sentinels[id.Name] {
				return true
			}
			// Count the door's callers, wherever they are.
			if call, ok := n.(*ast.CallExpr); ok {
				if name := doorCallee(call.Fun); name == "MissingToolFor" {
					doorCalls++
					doorCallers[rel] = true
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	if files < 50 {
		t.Fatalf("the scan read %d files — it is not reading the tree", files)
	}

	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("a missing-converter refusal classified outside the door: %s\n\t"+
			"Call pdfops.MissingToolFor(err) and word the refusal for this surface, rather than "+
			"matching the sentinel here. ADR-009/ADR-040: one door, and each site says it in its "+
			"own voice.", o)
	}

	// ── The floor ────────────────────────────────────────────────────────────────────────────
	//
	// This guard's vacuity trap is specific and it is NOT "the input was empty": once the door
	// absorbs both sentinels, zero sites outside internal/pdfops mention them — which is the
	// success condition AND what a guard that had stopped matching anything would report. So a
	// clean `offenders` list proves nothing on its own. Two things make the pass mean something:
	// the sentinels still exist where the door can see them, and the door still has the callers
	// that used to be the copies.
	for name := range sentinels {
		if declaredIn[name] == "" {
			t.Errorf("sentinel %q is declared nowhere under %s — renamed or removed, and this "+
				"guard has been matching nothing", name, doorPkg)
		}
	}
	// The floor counts CALL SITES, not calling files, and the difference is not pedantry:
	// `internal/cli/commands.go` holds two of the four (cmdPDFA and cmdOffice), so a per-file
	// count reads 3 against a floor of 4 and reports a regression that has not happened.
	//
	// Written down because this guard's FIRST run was red for exactly that reason: the message
	// said "sites" while the predicate counted files. Nothing in the toolchain compares an
	// assertion's message against what it actually checks, so the only thing that catches it is
	// reading one against the other when it fires.
	if doorCalls < 4 {
		var who []string
		for f := range doorCallers {
			who = append(who, f)
		}
		sort.Strings(who)
		t.Errorf("pdfops.MissingToolFor has %d call sites across %d file(s) (%s); the four that "+
			"used to classify for themselves are two HTTP handlers and two CLI verbs. Fewer means "+
			"a site stopped routing through the door — or that this scan no longer finds calls at all",
			doorCalls, len(doorCallers), strings.Join(who, ", "))
	}
}

// doorCallee is the function name of a call, bare or through its package. Deliberately local
// rather than reusing authoringscan_test.go's calleeName: that scan is shared by two guards
// over ONE population (authoring primitives), and its own comment says sharing is right for
// that reason. A missing-converter refusal is a different population, so this is a sibling
// scan, not a second consumer of that one.
func doorCallee(fun ast.Expr) string {
	switch e := fun.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}
