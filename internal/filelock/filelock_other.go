//go:build !windows

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Try asks for an exclusive flock on f without waiting. got is false when another open file
// holds it. flock is per open file description and is released by the kernel when the last
// descriptor on it closes — on exit and on a crash alike.
func Try(f *os.File) (got bool, err error) {
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EWOULDBLOCK):
			return false, nil
		default:
			return false, err
		}
	}
}

// Unlock releases a lock Try took. Closing the file releases it too; this is for a holder that
// keeps the file open.
func Unlock(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
