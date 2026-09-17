package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestReplaceDurableWritesThroughASymlinkInsteadOfReplacingIt — /pending 515.
//
// Every door in this package finishes with a rename, and `rename(tmp, link)` replaces THE LINK.
// So a user who keeps `~/contract.pdf` pointing at a file in a synced folder, opens it and saves
// got: a regular file where her link was, holding the new bytes; the real document still holding
// the old ones; and no message about either. `internal/cli`'s `writeNamed` had carried the fix
// since /pending 504 and the GUI's two save doors had never had it.
//
// The assertion is on BOTH halves, and the pair is the point: a fix that wrote the target but
// still clobbered the link, or kept the link and wrote nothing, passes half of this.
func TestReplaceDurableWritesThroughASymlinkInsteadOfReplacingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a symlink needs a privilege this test cannot assume on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.pdf")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.pdf")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("this filesystem will not make a symlink: %v", err)
	}
	// STIMULUS: it really is a link before the write, so "still a link" below is an assertion
	// rather than a description of a plain file that was never one.
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("setup: %s is not a symlink (%v, %v)", link, fi, err)
	}

	if err := ReplaceDurable(link, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the write replaced the user's symlink with a regular file (%v, %v) — her link "+
			"is gone and the document it pointed at was not written", fi.Mode(), err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new" {
		t.Errorf("the file the link points at holds %q (%v), want \"new\" — the write landed "+
			"somewhere other than the document the user was editing", got, err)
	}
}

// TestReplaceDurableFollowsAChainAndLeavesALinkItCannotResolve — the three shapes the simple case
// does not cover, asserted together because one line of production code decides all three.
//
// A CHAIN resolves to the file at the end of it: EvalSymlinks walks the whole chain, and stopping
// at the first hop would replace the second link exactly as the original defect replaced the first.
// A DANGLING link and a LOOP have no target to write to, so the bytes land on the link's own name —
// which is the only thing that can succeed, and is what happened before this existed.
func TestReplaceDurableFollowsAChainAndLeavesALinkItCannotResolve(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a symlink needs a privilege this test cannot assume on Windows")
	}
	dir := t.TempDir()

	target := filepath.Join(dir, "end.pdf")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	mid, head := filepath.Join(dir, "mid.pdf"), filepath.Join(dir, "head.pdf")
	if err := os.Symlink(target, mid); err != nil {
		t.Skipf("this filesystem will not make a symlink: %v", err)
	}
	if err := os.Symlink(mid, head); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceDurable(head, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new" {
		t.Errorf("a two-hop chain landed %q at its end, want \"new\" — the write stopped short "+
			"and replaced a link on the way", got)
	}
	for _, l := range []string{head, mid} {
		if fi, err := os.Lstat(l); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is no longer a symlink after the write (%v)", filepath.Base(l), err)
		}
	}

	// Dangling: nothing to resolve, so the link's own name takes the bytes rather than the write
	// failing on a target that was never there.
	dangling := filepath.Join(dir, "dangling.pdf")
	if err := os.Symlink(filepath.Join(dir, "nothing-here.pdf"), dangling); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceDurable(dangling, []byte("d"), 0o600); err != nil {
		t.Errorf("a dangling link refused the write (%v) — the user's save would fail with no "+
			"file anywhere to show for it", err)
	} else if got, _ := os.ReadFile(dangling); string(got) != "d" {
		t.Errorf("a dangling link holds %q after the write, want \"d\"", got)
	}

	// A loop: EvalSymlinks returns ELOOP, and the same fallback applies.
	a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
	if err := os.Symlink(b, a); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceDurable(a, []byte("l"), 0o600); err != nil {
		t.Errorf("a symlink loop refused the write (%v) — a save must not fail on one", err)
	} else if got, _ := os.ReadFile(a); string(got) != "l" {
		t.Errorf("a looped link holds %q after the write, want \"l\"", got)
	}
}
