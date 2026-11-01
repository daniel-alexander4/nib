package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestNoClobberRefusesToReplaceAnEarlierExtractionWholly — RelaxTax B7. A rerun of the CSV mail-merge (or a
// split) over a folder holding the earlier parts replaced them all with exit 0; with --no-clobber it refuses
// before the first write, so the folder is never half old and half new.
func TestNoClobberRefusesToReplaceAnEarlierExtractionWholly(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "form.pdf")
	os.WriteFile(in, form, 0o600)
	csv1 := filepath.Join(dir, "a.csv")
	os.WriteFile(csv1, []byte("fullName,id\nAda,one\nGrace,two\n"), 0o600)
	out := filepath.Join(dir, "out")

	run := func(extra ...string) (string, int) {
		_, stderr, code := fillChild(t, "", "", append([]string{"fill", in, "--data", csv1, "--out-dir", out, "--name-col", "id"}, extra...)...)
		return stderr, code
	}
	if stderr, code := run("--no-clobber"); code != 0 {
		t.Fatalf("first run: exit %d: %s", code, stderr)
	}
	first, _ := os.ReadFile(filepath.Join(out, "one.pdf"))
	if len(first) == 0 {
		t.Fatal("the first run wrote nothing: the test reaches nothing")
	}
	// A third file would be new; the refusal must come before it is written even though it would not clash.
	os.WriteFile(csv1, []byte("fullName,id\nAda,one\nGrace,two\nNew,three\n"), 0o600)
	stderr, code := run("--no-clobber")
	if code != 1 || !strings.Contains(stderr, "already exists") || !strings.Contains(stderr, "nothing was written") {
		t.Errorf("rerun with --no-clobber: exit %d, stderr %q, want 1 and the refusal", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(out, "three.pdf")); err == nil {
		t.Error("a new file was written beside the refused clashing ones: the folder is half old and half new")
	}
	// Without the flag the old behaviour stands (an addition, nothing else changed).
	if stderr, code := run(); code != 0 {
		t.Errorf("rerun without the flag: exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(out, "three.pdf")); err != nil {
		t.Errorf("without the flag the third file should be written: %v", err)
	}
}

func TestNoClobberOnSplit(t *testing.T) {
	pdf, err := testpdf.Text("one", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.pdf")
	os.WriteFile(in, pdf, 0o600)
	out := filepath.Join(dir, "parts")
	args := func(extra ...string) []string {
		return append([]string{"split", in, "--every", "1", "--out-dir", out}, extra...)
	}
	if _, stderr, code := fillChild(t, "", "", args("--no-clobber")...); code != 0 {
		t.Fatalf("first split: exit %d: %s", code, stderr)
	}
	if _, stderr, code := fillChild(t, "", "", args("--no-clobber")...); code != 1 || !strings.Contains(stderr, "already exists") {
		t.Errorf("second split with --no-clobber: exit %d, stderr %q", code, stderr)
	}
	if _, _, code := fillChild(t, "", "", args()...); code != 0 {
		t.Errorf("second split without the flag: exit %d, want the old behaviour (0)", code)
	}
}
