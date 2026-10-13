//go:build !linux

package server

import "net/netip"

// socketOwner cannot be answered here, and says so (ADR-157's declared gap). The Linux sibling asks
// the kernel through NETLINK_SOCK_DIAG; macOS and Windows have no call that names the owner of a
// TCP socket by its addresses. What the gap costs differs by platform and is READ, not measured:
// on macOS `ps` shows every user's command lines, so the launch key is as exposed as it was on
// Linux; on Windows another user's command line needs a handle a standard user is not given.
func socketOwner(local, peer netip.AddrPort) (uint32, peerState) {
	return 0, peerUnknown
}
