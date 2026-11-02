//go:build !windows

package tempdir

import (
	"fmt"
	"os"
	"syscall"
)

// modeOK: closed to everyone but the owner.
func modeOK(fi os.FileInfo) bool { return fi.Mode().Perm() == 0o700 }

func ownedByUs(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

// checkPrivate: the root is a real directory, ours, and closed to others. A mode we can tighten on
// a directory we own is tightened; anything else is refused.
func checkPrivate(p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || !ownedByUs(fi) {
		return fmt.Errorf("%w: %s", ErrUnsafeRoot, p)
	}
	if fi.Mode().Perm() != 0o700 {
		if err := os.Chmod(p, 0o700); err != nil {
			return fmt.Errorf("%w: %s", ErrUnsafeRoot, p)
		}
	}
	return nil
}
