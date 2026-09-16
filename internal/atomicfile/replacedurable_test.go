package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestReplaceDurableKeepsAnExistingFilesModeAndNamesANewOnes — /pending 499.
func TestReplaceDurableKeepsAnExistingFilesModeAndNamesANewOnes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows carries only the read-only bit; there is no group/other mode to lose")
	}
	dir := t.TempDir()

	existing := filepath.Join(dir, "shared.pdf")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o644); err != nil { // past the umask
		t.Fatal(err)
	}
	if err := ReplaceDurable(existing, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(existing)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(existing); string(got) != "new" {
		t.Fatalf("setup: the replacement did not land (%q), so the mode below describes nothing", got)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("replacing a 0644 file left it %v — the user's group/other read access was "+
			"silently removed by a save", fi.Mode().Perm())
	}

	fresh := filepath.Join(dir, "new.pdf")
	if err := ReplaceDurable(fresh, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(fresh); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("a NEW file should take the caller's perm 0600; got %v (%v)", fi.Mode().Perm(), err)
	}
}
