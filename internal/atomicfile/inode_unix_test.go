//go:build unix

package atomicfile

import (
	"os"
	"syscall"
)

// inode returns the file's inode number where the platform exposes one.
func inode(fi os.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Ino), true
}
