package tempdir

import "nib/internal/filelock"

// resetRoot forgets the cached root so a test can point TMPDIR at its own directory.
func resetRoot() {
	rootMu.Lock()
	root = ""
	rootMu.Unlock()
}

// releaseByHand drops a directory's lock as the kernel does when its holder dies.
func releaseByHand(d *Dir) {
	filelock.Unlock(d.live)
	_ = d.live.Close()
}
