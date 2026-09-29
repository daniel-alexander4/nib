package rendezvous

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOnlyTestsOpenAdmittingLoopback — the widened cache rule is a test seam, and a production
// caller would reopen /pending 707: a loopback stranger's ping would count as a cache again.
// Walks the module's non-test Go files; the one declaration is the only non-test mention allowed.
func TestOnlyTestsOpenAdmittingLoopback(t *testing.T) {
	root := filepath.Join("..", "..")
	var files int
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".claude" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		files++
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		n := strings.Count(string(b), "OpenAdmittingLoopback(")
		if filepath.Base(p) == "dht.go" && strings.HasSuffix(filepath.Dir(p), "rendezvous") {
			n -= strings.Count(string(b), "func OpenAdmittingLoopback(")
		}
		if n > 0 {
			t.Errorf("%s calls rendezvous.OpenAdmittingLoopback — production must open through Open, "+
				"whose cache rule refuses loopback (/pending 707)", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 {
		t.Fatalf("setup: walked only %d non-test Go files — the census is not seeing the module", files)
	}
}
