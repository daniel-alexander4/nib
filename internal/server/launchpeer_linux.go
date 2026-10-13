//go:build linux

package server

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"syscall"
)

// The kernel's own answer to "whose socket is this" (ADR-157): one exact lookup over
// NETLINK_SOCK_DIAG, the interface `ss` reads. An exact lookup, not a walk of `/proc/net/tcp` —
// that file is read in pieces while the table changes under it, so a row can be skipped, and it
// costs a line per socket on the machine.
const (
	netlinkSockDiag  = 4  // NETLINK_SOCK_DIAG
	sockDiagByFamily = 20 // SOCK_DIAG_BY_FAMILY
	nlmsgError       = 2  // NLMSG_ERROR
	nlmFRequest      = 1  // NLM_F_REQUEST
	nlHdrLen         = 16 // struct nlmsghdr
	diagReqLen       = 56 // struct inet_diag_req_v2
	diagMsgLen       = 72 // struct inet_diag_msg
	diagIDOff        = 4  // inet_diag_sockid inside inet_diag_msg (8 inside the request)
	diagUIDOff       = 64 // idiag_uid inside inet_diag_msg
	diagInodeOff     = 68 // idiag_inode inside inet_diag_msg
	tcpTimeWait      = 6
	tcpListen        = 10
)

// socketOwner reports the user owning the socket whose own address is peer and whose far end is
// local — the other end of a connection this process accepted.
//
// **It asks about this process's OWN end first.** That socket certainly exists, so a kernel that
// cannot find it is one whose answers mean nothing here (a sandbox that implements the call and
// not the table), and "no such socket" for the far end must not then read as "gone".
func socketOwner(local, peer netip.AddrPort) (uint32, peerState) {
	if local.Addr().Is4() != peer.Addr().Is4() {
		return 0, peerUnknown
	}
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, netlinkSockDiag)
	if err != nil {
		return 0, peerUnknown
	}
	defer syscall.Close(fd)
	// A kernel that never answers must not hold the window's first request for ever.
	if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{Sec: 1}); err != nil {
		return 0, peerUnknown
	}
	// **The question is asked as the kernel asks it when it delivers a packet**: for the socket
	// that would receive what arrives for these four numbers ON THE LOOPBACK INTERFACE. Asked with
	// no interface, a socket its owner has bound to `lo` (SO_BINDTODEVICE needs no privilege) is
	// not found — measured — and would have read as "gone".
	for _, ifindex := range loopbackInterfaces() {
		if _, state := diagOwner(fd, local, peer, ifindex); state != peerFound {
			continue
		}
		return diagOwner(fd, peer, local, ifindex)
	}
	return 0, peerUnknown
}

// loopbackInterfaces lists the interfaces a loopback connection's packets arrive on — `lo`.
func loopbackInterfaces() []uint32 {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []uint32
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback != 0 && i.Index > 0 {
			out = append(out, uint32(i.Index))
		}
	}
	return out
}

// diagOwner asks for the one TCP socket bound to src and connected to dst that receives on ifindex.
func diagOwner(fd int, src, dst netip.AddrPort, ifindex uint32) (uint32, peerState) {
	ne := binary.NativeEndian
	req := make([]byte, nlHdrLen+diagReqLen)
	ne.PutUint32(req[0:], uint32(len(req)))
	ne.PutUint16(req[4:], sockDiagByFamily)
	ne.PutUint16(req[6:], nlmFRequest)
	ne.PutUint32(req[8:], 1) // sequence
	p := req[nlHdrLen:]
	p[0] = syscall.AF_INET
	if !src.Addr().Is4() {
		p[0] = syscall.AF_INET6
	}
	p[1] = syscall.IPPROTO_TCP
	id := p[8:] // struct inet_diag_sockid: ports and addresses in network order
	binary.BigEndian.PutUint16(id[0:], src.Port())
	binary.BigEndian.PutUint16(id[2:], dst.Port())
	copy(id[4:20], src.Addr().AsSlice())
	copy(id[20:36], dst.Addr().AsSlice())
	ne.PutUint32(id[36:], ifindex)
	for i := 40; i < 48; i++ {
		id[i] = 0xff // INET_DIAG_NOCOOKIE
	}
	if err := syscall.Sendto(fd, req, 0, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return 0, peerUnknown
	}
	buf := make([]byte, 4096)
	n, from, err := syscall.Recvfrom(fd, buf, 0)
	if err != nil || n < nlHdrLen {
		return 0, peerUnknown
	}
	// Only the kernel's answer counts.
	if nl, ok := from.(*syscall.SockaddrNetlink); !ok || nl.Pid != 0 {
		return 0, peerUnknown
	}
	body := buf[nlHdrLen:n]
	switch ne.Uint16(buf[4:]) {
	case nlmsgError:
		if len(body) >= 4 && syscall.Errno(-int32(ne.Uint32(body))) == syscall.ENOENT {
			return 0, peerGone
		}
		return 0, peerUnknown
	case sockDiagByFamily:
		if len(body) < diagMsgLen {
			return 0, peerUnknown
		}
		// With no connection on these four numbers the kernel answers with a LISTENER on the
		// source address instead; that is somebody else's socket, and a closed connection waiting
		// out its timer reports no owner at all. Neither is the far end of a live connection.
		if state := body[1]; state == tcpListen || state == tcpTimeWait {
			return 0, peerGone
		}
		// The answer must be about the socket that was asked for.
		if !bytes.Equal(body[diagIDOff:diagIDOff+36], id[:36]) {
			return 0, peerGone
		}
		// A socket nobody holds any more — closed by its owner, still finishing — has no inode,
		// and the kernel reports its owner as 0. That is not root.
		if ne.Uint32(body[diagInodeOff:]) == 0 {
			return 0, peerGone
		}
		return ne.Uint32(body[diagUIDOff:]), peerFound
	}
	return 0, peerUnknown
}
