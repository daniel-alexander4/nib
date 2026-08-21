//go:build !windows

package discovery

import "syscall"

// setReuseAddr sets SO_REUSEADDR on fd so two listeners can share the discovery port.
//
// It exists as a two-file shim because syscall.SetsockoptInt takes an `int` here and a
// syscall.Handle on Windows, so the single inline call that used to be in mcast.go meant
// **nib did not compile for Windows at all** — `GOOS=windows go build ./cmd/nib` failed,
// so nib.exe could not be produced, on the platform whose `nib register` command exists
// only for it.
//
// mcast.go's own note argued against "a //go:build file per platform" because "a no-op
// sibling is the shape that already shipped one silent defect here (ReplaceOthers returning
// 0 off Linux)". That reasoning was right about the hazard and produced a worse outcome
// than the hazard: not a silent no-op, but no binary. Both files here do the real thing, and
// TestBothPlatformsSetReuseAddr asserts neither is a stub.
func setReuseAddr(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}
