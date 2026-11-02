//go:build windows

package tempdir

import "os"

// modeOK: Windows reports no meaningful mode bits; the ACL is the check and is a declared gap.
func modeOK(fi os.FileInfo) bool { return true }

// ownedByUs: a Windows temp directory is already per-user; ownership is the ACL's and is not read here
// (a declared gap, ADR-185).
func ownedByUs(fi os.FileInfo) bool { return true }

// checkPrivate: the directory must still be a real directory.
func checkPrivate(p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafeRoot
	}
	return nil
}
