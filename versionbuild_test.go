package nib

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAPlainBuildReportsTheVersionFile — ADR-183. `go build ./cmd/nib` sets no -ldflags, and it used to print
// "nib dev"; a program that names a minimum version cannot read that. The embedded VERSION is the answer, so it
// is also what build.sh's -X sets: they cannot disagree.
func TestAPlainBuildReportsTheVersionFile(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	want, err := os.ReadFile("VERSION")
	if err != nil {
		t.Skip("no VERSION file (a vendored copy)")
	}
	if VersionFile() != strings.TrimSpace(string(want)) {
		t.Fatalf("VersionFile() = %q, VERSION file = %q", VersionFile(), want)
	}
	exe := filepath.Join(t.TempDir(), "nib")
	if out, err := exec.Command("go", "build", "-o", exe, "./cmd/nib").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, err := exec.Command(exe, "version", "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil || got["version"] != VersionFile() || got["version"] == "dev" || got["api"] == nil {
		t.Errorf("a plain build says %s (%v), want version %q and an api number", out, err, VersionFile())
	}
}
