package server

import (
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
)

// Who is trading a launch key (ADR-157, `/pending 701`).
//
// **The key is in the browser's command line from launch until the page trades it** — `browser.Open`
// passes `--app=http://127.0.0.1:P/#k=KEY` — and a command line is readable by every user of the
// machine (`/proc/<pid>/cmdline` is 0444). Measured: another process read the key 118 ms after the
// browser started and traded it; the real window, on a new profile, came 5.6 s later and was refused.
// TCP loopback is not per-user, so the trade route could not tell the two apart.
//
// **The kernel can.** Every socket records the user that made it, and the connection a trade arrives
// on has a far end in this machine's own table. So the trade asks who owns it, and a key offered by
// ANOTHER user is refused without being spent: the window it was minted for still trades it. The key
// stays in the command line; what goes is its use to anyone but the user it was minted for.
//
// **One answer refuses, and only that one**: the kernel names an owner who is neither this user nor
// root. Everything else trades as it always did —
//
//   - root: it reads this process's memory already, and a relay root runs is root's to vouch for;
//   - no such socket (`peerGone`): the far end is not a socket of this machine — a peer reached
//     through address translation that only root can set up — or it has closed and cannot be answered;
//   - the kernel will not say (`peerUnknown`): another platform, or a sandbox without the socket
//     diagnostics. Said once in the log, because there the command line is as exposed as it was.
//
// A Nib that does not open is worse than the gap, so nothing here refuses on a doubt.
type peerState int

const (
	peerUnknown peerState = iota // the kernel would not answer, or this platform cannot ask
	peerGone                     // no live socket of this machine is the far end
	peerFound                    // the far end is a socket of this machine, owned by the returned user
)

// ownerOfPeer reports who owns the far end of the connection r arrived on. A field on the Server
// (`launchPeerOwner`) stands in for it in tests; `socketOwner` is the platform's answer.
func (s *Server) ownerOfPeer(r *http.Request) (uint32, peerState) {
	local, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if local == nil {
		return 0, peerUnknown
	}
	l, err1 := netip.ParseAddrPort(local.String())
	p, err2 := netip.ParseAddrPort(r.RemoteAddr)
	if err1 != nil || err2 != nil {
		return 0, peerUnknown
	}
	l = netip.AddrPortFrom(l.Addr().Unmap().WithZone(""), l.Port())
	p = netip.AddrPortFrom(p.Addr().Unmap().WithZone(""), p.Port())
	if s.launchPeerOwner != nil {
		return s.launchPeerOwner(l, p)
	}
	return socketOwner(l, p)
}

// tradedByAnotherUser is the ONE question the trade asks about its caller: does the kernel name
// someone other than this user or root as the owner of the connection?
func (s *Server) tradedByAnotherUser(r *http.Request) bool {
	uid, state := s.ownerOfPeer(r)
	switch state {
	case peerFound:
		me := uint32(os.Geteuid())
		if uid == me || uid == 0 {
			return false
		}
		log.Printf("refused a launch key offered by user %d: this Nib runs as user %d (ADR-157)", uid, me)
		return true
	case peerUnknown:
		s.peerUnknownOnce.Do(func() {
			log.Printf("cannot tell which user trades a launch key on this system; a key read from the browser's command line by another user could be traded by them (ADR-157)")
		})
	}
	return false
}
