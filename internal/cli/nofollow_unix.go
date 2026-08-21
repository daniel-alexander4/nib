//go:build !windows

package cli

import "syscall"

// oNoFollow makes an open refuse a symlink, closing the window between scanOnce's Lstat and
// the action that reads the file. See readNoFollow.
//
// **O_NONBLOCK rides along, and it is not incidental.** O_NOFOLLOW refuses a symlink and not
// a fifo, and opening a fifo for reading BLOCKS until a writer appears — so the
// regular-file check readNoFollow does on the handle never ran, and a `pipe.pdf` dropped
// into a watched directory hung the whole watch loop until Ctrl-C. Measured: the first draft
// of TestTheWatchRefusesToReadThroughASymlink timed out at five seconds against exactly
// that. With O_NONBLOCK the open returns immediately and the check gets to refuse it. On a
// regular file the flag does nothing.
const oNoFollow = syscall.O_NOFOLLOW | syscall.O_NONBLOCK
