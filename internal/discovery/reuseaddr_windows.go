//go:build windows

package discovery

import "syscall"

// setReuseAddr sets SO_REUSEADDR on fd. See the note in reuseaddr_unix.go for why this is a
// two-file shim rather than one inline call.
//
// **Windows' SO_REUSEADDR is not Unix's, and that is worth knowing before relying on it.**
// On Unix it permits a second bind to the same multicast address; on Windows it permits a
// second bind to the same address *generally*, which is the weaker guarantee. It is the
// option Windows offers for sharing a multicast port and it is what every other multicast
// implementation uses there, so it is the right call — but discovery's socket is already
// treated as internet-facing on every platform (ADR-007), which is the property that makes
// the difference not matter: nothing here trusts the socket's exclusivity for anything.
func setReuseAddr(fd uintptr) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}
