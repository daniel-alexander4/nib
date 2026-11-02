package nib

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempCreators finds every call that makes a temporary file or directory, or names the system temp
// directory, in one source file: the shapes a second door onto temp space takes.
func tempCreators(t *testing.T, name, src string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch pkg.Name + "." + sel.Sel.Name {
		case "os.MkdirTemp", "os.CreateTemp", "os.TempDir", "ioutil.TempDir", "ioutil.TempFile":
			hits = append(hits, pkg.Name+"."+sel.Sel.Name)
		}
		return true
	})
	return hits
}

// TestTheSeededTempScanSeesWhatItGuards plants each way a temp directory is made outside the door,
// so the census below is shown to fire rather than assumed to.
func TestTheSeededTempScanSeesWhatItGuards(t *testing.T) {
	planted := map[string]string{
		"mkdir":  `package x; import "os"; func f() { os.MkdirTemp("", "nib-evil-") }`,
		"create": `package x; import "os"; func f() { os.CreateTemp("", "x") }`,
		"tmpdir": `package x; import "os"; func f() string { return os.TempDir() }`,
		"ioutil": `package x; import "io/ioutil"; func f() { ioutil.TempDir("", "x") }`,
	}
	for k, src := range planted {
		if len(tempCreators(t, k+".go", src)) == 0 {
			t.Errorf("the scan does not see the planted %s", k)
		}
	}
	if got := tempCreators(t, "ok.go", `package x; import "os"; func f() { os.ReadFile("x"); tempdir.New("office") }`); len(got) != 0 {
		t.Errorf("the scan flags code that makes no temp space: %v", got)
	}
}

// TestEveryTempDirectoryComesThroughTheDoor — ADR-185, ADR-009. A temp directory made anywhere but
// `internal/tempdir` has no liveness lock, so the startup sweep can never tell a crashed run's from a
// running one's, and it sits in the shared system temp directory under a name another user can
// plant. Four such calls existed (`nib-office-`, `nib-pdfa-`, `nib-pdfcpu-`, `nib-rendezvous-*`);
// all four are the door's now.
func TestEveryTempDirectoryComesThroughTheDoor(t *testing.T) {
	declared := map[string]string{
		"internal/tempdir/tempdir.go": "the door",
		// `os.CreateTemp(dir, ".nib-*.tmp")` in the TARGET'S OWN directory, the first half of an atomic
		// rename: it must share the destination's filesystem, so it can never live in a temp root.
		"internal/atomicfile/atomicfile.go": "the temp half of an atomic write, in the destination's directory",
	}
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "web", "docs", "test", "third_party", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 60 {
		t.Fatalf("setup: only %d source files found — this guard is not walking the repo", len(files))
	}
	used := map[string]bool{}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		key := filepath.ToSlash(path)
		if hits := tempCreators(t, path, src); len(hits) > 0 {
			if _, ok := declared[key]; !ok {
				t.Errorf("%s calls %v and is not the temp door.\nRoute it through tempdir.New (liveness lock, per-user 0700 root, swept at start) or declare it here with the reason.", key, hits)
			}
			used[key] = true
		}
	}
	for k := range declared {
		if !used[k] {
			t.Errorf("%s is declared as an exemption and makes no temp space any more; remove the entry", k)
		}
	}
}
