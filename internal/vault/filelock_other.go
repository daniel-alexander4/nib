//go:build !windows

package vault

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock asks for an exclusive flock on f without waiting. got is false when another open file
// holds it. flock is per open file description and is released by the kernel when the last
// descriptor on it closes — on exit and on a crash alike.
func tryLock(f *os.File) (got bool, err error) {
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

func unlock(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
