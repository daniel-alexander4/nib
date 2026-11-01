package ceremony

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `RemoveEnded` — ADR-180, `/pending 874`. It deletes a closed-out ceremony's folder (and the numbered ones set
// aside beside it) and nothing else.

const (
	idA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func mkFolder(t *testing.T, root string, sub ...string) string {
	t.Helper()
	dir := filepath.Join(append([]string{root}, sub...)...)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "document.pdf"), []byte("the signed document"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func TestRemoveEndedDeletesTheEndedFoldersOfThatIdAndNothingElse(t *testing.T) {
	root := t.TempDir()
	endedA := mkFolder(t, root, "ended", idA)
	setAside := mkFolder(t, root, "ended", idA+".2")
	endedB := mkFolder(t, root, "ended", idB)
	liveA := mkFolder(t, root, "ceremonies", idA) // a proceeding that took the same id after the close-out
	for _, p := range []string{endedA, setAside, endedB, liveA} {
		if !exists(p) {
			t.Fatalf("setup: %s is missing, so what survives below proves nothing", p)
		}
	}

	n, err := RemoveEnded(root, idA)
	if err != nil || n != 2 {
		t.Fatalf("RemoveEnded = %d, %v; want 2 folders and no error", n, err)
	}
	if exists(endedA) || exists(setAside) {
		t.Error("an ended folder for that id survived")
	}
	if !exists(endedB) {
		t.Error("another ceremony's ended folder was removed")
	}
	if !exists(liveA) {
		t.Error("the LIVE folder with the same id was removed: ended/ is all this reaches")
	}
}

func TestRemoveEndedRefusesWhatIsNotAnEndedCeremony(t *testing.T) {
	root := t.TempDir()
	keep := mkFolder(t, root, "ended", idB)
	live := mkFolder(t, root, "ceremonies", idA)

	if _, err := RemoveEnded(root, idA); !errors.Is(err, ErrNotEnded) {
		t.Errorf("a live-only id = %v, want ErrNotEnded", err)
	}
	if !exists(live) {
		t.Error("a refused removal deleted the live folder")
	}
	for _, bad := range []string{"", "../ended/" + idB, idB + "/..", "ended", strings.Repeat("g", 32), idB + ".2"} {
		if _, err := RemoveEnded(root, bad); err == nil {
			t.Errorf("id %q was accepted", bad)
		}
	}
	if !exists(keep) {
		t.Error("a malformed id reached an ended folder")
	}
	if _, err := RemoveEnded("relative/root", idB); !errors.Is(err, ErrRootNotAbsolute) {
		t.Errorf("a relative root = %v, want ErrRootNotAbsolute", err)
	}
}

func TestRemoveEndedDoesNotFollowALink(t *testing.T) {
	root := t.TempDir()
	elsewhere := mkFolder(t, t.TempDir(), "precious")
	if err := os.MkdirAll(filepath.Join(root, "ended"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(root, "ended", idA)); err != nil {
		t.Skipf("SKIP (not a pass): symlinks unavailable here: %v", err)
	}
	if _, err := RemoveEnded(root, idA); !errors.Is(err, ErrNotEnded) {
		t.Errorf("a link named like an ended folder = %v, want ErrNotEnded", err)
	}
	if !exists(filepath.Join(elsewhere, "document.pdf")) {
		t.Error("the folder a link pointed at was deleted")
	}
}
