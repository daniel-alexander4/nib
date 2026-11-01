package nib

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAReleaseCarriesItsChecksums — RelaxTax N9 (ADR-183). A program that names a minimum version needs a
// way to know the file it downloaded is the one the build made. build.sh's checksum block is run here on a
// stand-in dist/ (never build.sh itself, which builds six targets and, with --publish, publishes), and the
// asset list it uploads must carry the file.
func TestAReleaseCarriesItsChecksums(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("no sha256sum (the block falls back to shasum on macOS)")
	}
	b, err := os.ReadFile("build.sh")
	if err != nil {
		t.Skip("no build.sh (a vendored copy)")
	}
	src := string(b)
	from := strings.Index(src, "# SHA256SUMS:")
	to := strings.Index(src, `echo "done`)
	if from < 0 || to < from {
		t.Fatal("build.sh has no SHA256SUMS block before its \"done\" line")
	}
	if !strings.Contains(src, `"$DIST/SHA256SUMS"`) {
		t.Error("the publish asset list does not carry $DIST/SHA256SUMS: a release would ship binaries with no checksums")
	}
	dist := t.TempDir()
	want := []string{"nib-9.9.9-linux-amd64", "nib-9.9.9-windows-amd64.exe", "nib_9.9.9_amd64.deb"}
	for _, n := range append(want, "LICENSE", "nib-8.8.8-linux-amd64") {
		if err := os.WriteFile(filepath.Join(dist, n), []byte("bytes of "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+src[from:to])
	cmd.Env = append(os.Environ(), "DIST="+dist, "VERSION=9.9.9")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the block failed: %v\n%s", err, out)
	}
	sums, err := os.ReadFile(filepath.Join(dist, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range want {
		if !strings.Contains(string(sums), "  "+n+"\n") {
			t.Errorf("SHA256SUMS does not list %s:\n%s", n, sums)
		}
	}
	for _, n := range []string{"LICENSE", "8.8.8"} {
		if strings.Contains(string(sums), n) {
			t.Errorf("SHA256SUMS lists %s, which is not this release's binary or package:\n%s", n, sums)
		}
	}
	check := exec.Command("sha256sum", "-c", "SHA256SUMS")
	check.Dir = dist
	if out, err := check.CombinedOutput(); err != nil {
		t.Errorf("the file does not verify what it lists: %v\n%s", err, out)
	}
}
