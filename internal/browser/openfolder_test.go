package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenFolderRefusesWhatIsNotAFolder — "a folder, never a file" is checked at the door
// (/pending 653). PATH is emptied so that, were the check gone, no opener could be started on the
// file: the test then fails on the sentence rather than by handing a file to the desktop.
func TestOpenFolderRefusesWhatIsNotAFolder(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	file := filepath.Join(dir, "nib_1.0.0_amd64.deb")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{file, filepath.Join(dir, "absent")} {
		err := OpenFolder(p)
		if err == nil || !strings.Contains(err.Error(), "not a folder") {
			t.Errorf("OpenFolder(%q) = %v, want a refusal saying it is not a folder", p, err)
		}
	}
}
