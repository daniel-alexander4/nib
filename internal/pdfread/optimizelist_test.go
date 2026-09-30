package pdfread

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// commandsNamedIn is the set of `model.X` names inside function fn of the Go file at path.
func commandsNamedIn(t *testing.T, path, fn string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	found := false
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn || fd.Recv != nil {
			continue
		}
		found = true
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if se, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := se.X.(*ast.Ident); ok && id.Name == "model" {
					set[se.Sel.Name] = true
				}
			}
			return true
		})
	}
	if !found {
		t.Fatalf("setup: %s declares no func %s — pdfcpu moved or renamed the list this guard reads; re-read "+
			"ReadValidateAndOptimize and restate what it consults", path, fn)
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestTheOptimizeListIsPdfcpusOwn — `/pending 715`: `cmdAssumingOptimization` restates pdfcpu's unexported list of
// the commands that optimize whatever `conf.Optimize` says, because `ReadOptimized` replaces the function that
// consults it. A pdfcpu that adds a command to its list would leave nib reading that command's context
// unoptimized, and one that drops a command would leave nib running a pass pdfcpu no longer runs; neither fails
// anything else. This reads the list out of the pdfcpu source the module builds against, not a version number,
// so an upgrade that leaves the list alone passes and one that moves it fails here.
func TestTheOptimizeListIsPdfcpusOwn(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/pdfcpu/pdfcpu").Output()
	if err != nil {
		t.Fatalf("setup: go list could not locate pdfcpu's source: %v", err)
	}
	dir := strings.TrimSpace(string(out))
	theirs := commandsNamedIn(t, filepath.Join(dir, "pkg", "api", "api.go"), "cmdAssumingOptimization")
	ours := commandsNamedIn(t, "optimize.go", "cmdAssumingOptimization")
	// Stimulus: the list is read, not an empty set that agrees with an empty set.
	if len(theirs) < 2 {
		t.Fatalf("setup: read %d commands from pdfcpu's list (%v) — the parse is not reading it", len(theirs), theirs)
	}
	if strings.Join(theirs, ",") != strings.Join(ours, ",") {
		t.Errorf("cmdAssumingOptimization names %v, and pdfcpu's own list in %s names %v — restate it", ours, dir, theirs)
	}
}
