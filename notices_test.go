package nib

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// TestNoticesUpToDate guards against THIRD-PARTY-NOTICES.md drifting from what
// build/gen-notices.sh would produce — the failure that let the pdfcpu version
// (and jsdiff attribution) rot for months. It regenerates the notices to a temp
// file and fails if they differ from the committed file, forcing a regenerate to
// land in the same commit as any dependency or vendored-asset change.
//
// The generator shells out to bash + go, so the test skips cleanly where those
// aren't available (and under -short) rather than failing spuriously; in Nib's
// normal `go test ./...` it runs.
func TestNoticesUpToDate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping notices freshness guard under -short")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available; skipping notices freshness guard")
	}

	got := filepath.Join(t.TempDir(), "notices.md")
	cmd := exec.Command("bash", "build/gen-notices.sh", got)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("gen-notices.sh failed: %v\n%s", err, stderr.String())
	}

	generated, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("THIRD-PARTY-NOTICES.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, committed) {
		t.Errorf("THIRD-PARTY-NOTICES.md is out of date (differs at %s) — run `make notices` and commit the result",
			firstDiff(committed, generated))
	}
}

// firstDiff returns a human-readable location of the first line that differs
// between the committed and generated notices, to point the fix at the change.
func firstDiff(a, b []byte) string {
	la, lb := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	for i := 0; i < len(la) && i < len(lb); i++ {
		if !bytes.Equal(la[i], lb[i]) {
			return "line " + strconv.Itoa(i+1)
		}
	}
	if len(la) != len(lb) {
		return "line " + strconv.Itoa(min(len(la), len(lb))+1) + " (length differs)"
	}
	return "an unknown line"
}
