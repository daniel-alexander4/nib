package nib

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// The generator runs `go list -deps ./cmd/nib`, whose module set is GOOS- and
	// build-tag-dependent, and the comparison below is byte-for-byte. The committed
	// file is generated on Linux, so on any other platform this fails on a file the
	// contributor did not touch and cannot fix — a spurious red that teaches people to
	// ignore the guard. Gated rather than made platform-agnostic: the file has to be
	// generated on SOME platform, and pinning that platform is the honest version of
	// "the committed artifact is the Linux one".
	if runtime.GOOS != "linux" {
		t.Skipf("THIRD-PARTY-NOTICES.md is generated on linux and `go list -deps` varies by GOOS; skipping on %s", runtime.GOOS)
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
