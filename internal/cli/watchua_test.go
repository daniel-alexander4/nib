package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// `nib watch DIR --do ua` and the refusal of `--do tag` — `PLAN-accessibility.md` P10.S03.

// TestWatchUAWritesTheReportNibUAPrints — the sidecar is `nib ua`'s table, byte for byte, followed by the
// sentences `nib ua` says on stderr; the document is not touched; the status agrees with `nib ua`'s exit.
func TestWatchUAWritesTheReportNibUAPrints(t *testing.T) {
	dir := t.TempDir()
	p := writePDF(t, dir, "in.pdf", "an untagged page")
	before := readPDF(t, p)

	var stdout string
	var code int
	stderr := captureStderr(t, func() { stdout, code = captureStdout(t, func() int { return cmdUA([]string{p}) }) })
	if code != 1 || strings.TrimSpace(stdout) == "" {
		t.Fatalf("setup: nib ua on an untagged document exited %d with table %q — want 1 and a table", code, stdout)
	}

	status, err := watchUA(p)
	if err != nil {
		t.Fatalf("watchUA: %v", err)
	}
	report, err := os.ReadFile(p + ".ua.txt")
	if err != nil {
		t.Fatalf("no sidecar: %v", err)
	}
	if !strings.HasPrefix(string(report), stdout) {
		t.Errorf("the sidecar does not begin with nib ua's table.\nnib ua:\n%s\nsidecar:\n%s", stdout, report)
	}
	notes := 0
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		line, _ = strings.CutPrefix(line, "nib: ")
		if line == "" {
			continue
		}
		notes++
		if !strings.Contains(string(report), line) {
			t.Errorf("the sidecar lacks nib ua's sentence %q:\n%s", line, report)
		}
	}
	if notes < 2 {
		t.Errorf("setup: nib ua said %d sentence(s) on stderr — want the provenance line and a verdict", notes)
	}
	if !strings.Contains(status, "not PDF/UA") {
		t.Errorf("status %q does not say the document is not PDF/UA, and nib ua exited 1", status)
	}
	if !bytes.Equal(before, readPDF(t, p)) {
		t.Error("writing the report changed the document")
	}
	// A temp file is created 0600; the report is an ordinary file beside the document.
	if info, err := os.Stat(p + ".ua.txt"); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Errorf("the sidecar's mode is %v, want 0644 — it kept the temp file's", info.Mode().Perm())
	}
}

// TestWatchUAReplacesAPlantedSymlinkRatherThanWritingThroughIt — the sidecar is written by rename, so a
// symlink someone dropped at FILE.ua.txt cannot redirect the report onto a file outside the directory.
func TestWatchUAReplacesAPlantedSymlinkRatherThanWritingThroughIt(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "precious.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := writePDF(t, dir, "in.pdf", "text")
	if err := os.Symlink(outside, p+".ua.txt"); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
	if _, err := watchUA(p); err != nil {
		t.Fatalf("watchUA: %v", err)
	}
	if got, _ := os.ReadFile(outside); string(got) != "keep" {
		t.Errorf("the report was written through the symlink: the outside file now reads %q", got)
	}
	info, err := os.Lstat(p + ".ua.txt")
	if err != nil || !info.Mode().IsRegular() {
		t.Errorf("the sidecar is not a regular file after the write: %v %v", info, err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".nib-*.tmp")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

// TestWatchRefusesTagNamingLaw3 — by name, before the directory is even looked at, and ua is an operation.
func TestWatchRefusesTagNamingLaw3(t *testing.T) {
	var code int
	stderr := captureStderr(t, func() { code = cmdWatch([]string{t.TempDir(), "--do", "tag"}) })
	if code != 1 || !strings.Contains(stderr, "law 3") || !strings.Contains(stderr, "nib tag commit") {
		t.Errorf("--do tag: exit %d, %q — want 1 naming law 3 and the reviewed path", code, stderr)
	}
	if watchOps["tag"] != nil {
		t.Error("watchOps offers tag")
	}
	if watchOps["ua"] == nil {
		t.Error("watchOps does not offer ua")
	}
}
