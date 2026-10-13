//go:build windows

package vault

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock asks for an exclusive LockFileEx on the first byte of f without waiting. got is false
// when another handle holds it. The lock belongs to the handle and Windows releases it when the
// handle closes — on exit and on a crash alike. The byte range may lie past the end of the file,
// so the empty lock file is locked as it is.
//
// Compiled and vetted for windows/amd64; not run on a Windows machine (ADR-154 declares it).
func tryLock(f *os.File) (got bool, err error) {
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION), errors.Is(err, windows.ERROR_IO_PENDING):
		return false, nil
	default:
		return false, err
	}
}

func unlock(f *os.File) {
	ol := new(windows.Overlapped)
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
