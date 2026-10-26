//go:build !unix

package atomicfile

import "os"

// inode: this platform exposes none, and the test that asks says so rather than passing on nothing.
func inode(os.FileInfo) (uint64, bool) { return 0, false }
