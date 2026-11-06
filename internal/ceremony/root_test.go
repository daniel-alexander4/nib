package ceremony

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInStoreNamesTheCeremonysWorkingFoldersOnly(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "ceremonies", "x", "doc.pdf")
	ended := filepath.Join(root, "ended", "x", "doc.pdf")
	beside := filepath.Join(root, "kept.pdf")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.MkdirAll(filepath.Join(root, "ceremonies", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "ceremonies", "x"), link); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, path string
		want       bool
	}{
		{"live", in, true}, {"ended", ended, true}, {"beside the store", beside, false},
		{"the store folder itself", filepath.Join(root, "ceremonies"), false},
		{"a link into the live folder", filepath.Join(link, "doc.pdf"), true},
		{"empty", "", false},
	} {
		if got := InStore(root, c.path); got != c.want {
			t.Errorf("%s: InStore = %v, want %v", c.name, got, c.want)
		}
	}
	if InStore("nib", in) {
		t.Error("a relative root must answer false, as storeDir refuses it")
	}
}
