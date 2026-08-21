package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/testpdf"
)

// TestWatchNeverFollowsASymlinkOutOfTheWatchedDirectory.
//
// `os.ReadDir`'s `DirEntry.Info()` is an **Lstat**, so a symlink named `x.pdf` passed the
// extension filter. `watchTransform` then read through it and `writeAtomic` — which calls
// `filepath.EvalSymlinks` — renamed over the TARGET. Anyone who can drop a file into the
// watched directory (the documented "process my inbox" and shared scan-drop uses) caused an
// unrequested in-place rewrite of any PDF elsewhere on disk the user can write, outside the
// directory the watch was pointed at. `--do sanitize` strips that document's metadata
// irreversibly.
func TestWatchNeverFollowsASymlinkOutOfTheWatchedDirectory(t *testing.T) {
	outside := t.TempDir()
	watched := t.TempDir()

	victim := filepath.Join(outside, "private.pdf")
	original, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, original, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(watched, "bait.pdf")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	// STIMULUS: the bait must look exactly like work to the scanner — right extension,
	// readable, and a real PDF at the other end. Otherwise the skip below proves nothing.
	if fi, err := os.Stat(link); err != nil || fi.Size() == 0 {
		t.Fatalf("setup: the symlink does not resolve to a readable PDF: %v", err)
	}

	seen := map[string]fileState{}
	processed := map[string]bool{}
	failed := map[string]fileState{}
	// TWICE, because scanOnce only acts on the SECOND sighting of an unchanged file —
	// "first sight or still changing — let it settle". A single call records state and
	// acts on nothing, so the first version of this test passed with the guard removed:
	// the traversal never happened because the action never ran.
	//
	// The real optimize action, so the write path under test is the shipped one.
	scanOnce(watched, seen, processed, failed, watchOps["optimize"])
	scanOnce(watched, seen, processed, failed, watchOps["optimize"])

	// STIMULUS: the settle logic really does act on the second pass. Proven against a
	// REGULAR file in the same directory, so a scanner that acted on nothing at all cannot
	// pass this test.
	control := filepath.Join(watched, "control.pdf")
	if err := os.WriteFile(control, original, 0o600); err != nil {
		t.Fatal(err)
	}
	cseen, cprocessed, cfailed := map[string]fileState{}, map[string]bool{}, map[string]fileState{}
	scanOnce(watched, cseen, cprocessed, cfailed, watchOps["optimize"])
	scanOnce(watched, cseen, cprocessed, cfailed, watchOps["optimize"])
	if !cprocessed[control] {
		t.Fatalf("setup: an ordinary PDF in the watched directory was not processed either "+
			"(failed=%v) — the scanner is acting on nothing and the assertion below is "+
			"about a traversal that never had a chance to happen", cfailed)
	}

	after, rerr := os.ReadFile(victim)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !bytes.Equal(after, original) {
		t.Errorf("a file OUTSIDE the watched directory was rewritten through a symlink "+
			"dropped inside it (%d bytes -> %d)", len(original), len(after))
	}
	if processed[link] {
		t.Error("the symlink was processed as a document")
	}
}
