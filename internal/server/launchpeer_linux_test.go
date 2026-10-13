//go:build linux

package server

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"os"
	"syscall"
	"testing"
	"time"
)

// The kernel's half of ADR-157, asked for real: no stand-in answers here.

// needSockDiag skips where this machine's kernel will not open the diagnostics socket at all (a
// sandbox). Anywhere it opens, every test below must hold.
func needSockDiag(t *testing.T) {
	t.Helper()
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, netlinkSockDiag)
	if err != nil {
		t.Skipf("no socket diagnostics on this machine: %v", err)
	}
	syscall.Close(fd)
}

func addrPort(t *testing.T, a net.Addr) netip.AddrPort {
	t.Helper()
	ap, err := netip.ParseAddrPort(a.String())
	if err != nil {
		t.Fatal(err)
	}
	return ap
}

// loopbackPair returns the accepted end of a new loopback connection and its dialling end.
func loopbackPair(t *testing.T, network, addr string, d net.Dialer) (accepted, dialled net.Conn) {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Skipf("cannot listen on %s: %v", addr, err)
	}
	t.Cleanup(func() { ln.Close() })
	dialled, err = d.DialContext(context.Background(), network, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dialled.Close() })
	accepted, err = ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { accepted.Close() })
	return accepted, dialled
}

// TestTheKernelNamesTheOwnerOfALoopbackPeer: this process's own connection to itself is owned by
// this user, over IPv4 and IPv6.
func TestTheKernelNamesTheOwnerOfALoopbackPeer(t *testing.T) {
	needSockDiag(t)
	for _, c := range []struct{ network, addr string }{{"tcp4", "127.0.0.1:0"}, {"tcp6", "[::1]:0"}} {
		a, _ := loopbackPair(t, c.network, c.addr, net.Dialer{})
		uid, state := socketOwner(addrPort(t, a.LocalAddr()), addrPort(t, a.RemoteAddr()))
		if state != peerFound || uid != uint32(os.Geteuid()) {
			t.Errorf("%s: owner of our own peer = (uid %d, state %d), want (uid %d, found)", c.network, uid, state, os.Geteuid())
		}
	}
}

// TestAPeerBoundToLoopbackIsStillNamed: a socket its owner has bound to `lo` — which takes no
// privilege — is invisible to a question asked with no interface. It read as "no such socket", and
// "no such socket" trades.
func TestAPeerBoundToLoopbackIsStillNamed(t *testing.T) {
	needSockDiag(t)
	bound := net.Dialer{Control: func(_, _ string, rc syscall.RawConn) error {
		var serr error
		if err := rc.Control(func(fd uintptr) {
			serr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, "lo")
		}); err != nil {
			return err
		}
		return serr
	}}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := bound.DialContext(context.Background(), "tcp4", ln.Addr().String())
	if err != nil {
		t.Skipf("this kernel will not bind an unprivileged socket to lo: %v", err)
	}
	defer c.Close()
	a, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	uid, state := socketOwner(addrPort(t, a.LocalAddr()), addrPort(t, a.RemoteAddr()))
	if state != peerFound || uid != uint32(os.Geteuid()) {
		t.Errorf("owner of a peer bound to lo = (uid %d, state %d), want (uid %d, found)", uid, state, os.Geteuid())
	}
}

// TestAClosedPeerIsGoneAndNotRoot: a socket its owner closed is reported by the kernel with owner
// 0 while it finishes. That is nobody, and must not read as root.
func TestAClosedPeerIsGoneAndNotRoot(t *testing.T) {
	needSockDiag(t)
	a, c := loopbackPair(t, "tcp4", "127.0.0.1:0", net.Dialer{})
	local, peer := addrPort(t, a.LocalAddr()), addrPort(t, a.RemoteAddr())
	c.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		uid, state := socketOwner(local, peer)
		if state == peerGone {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("owner of a closed peer = (uid %d, state %d), want gone", uid, state)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestARealTradeIsAskedOfTheKernelAndPasses: the whole path with nothing replaced — a request over
// loopback from this process is found, owned by this user, and trades.
func TestARealTradeIsAskedOfTheKernelAndPasses(t *testing.T) {
	needSockDiag(t)
	ts, srv := startServerWith(t)
	var uid uint32
	var state peerState
	srv.launchPeerOwner = func(l, p netip.AddrPort) (uint32, peerState) {
		uid, state = socketOwner(l, p)
		return uid, state
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", nil)
	req.Header.Set(headerLaunch, srv.MintLaunchKey())
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if state != peerFound || uid != uint32(os.Geteuid()) {
		t.Errorf("the kernel's answer for a real trade = (uid %d, state %d), want (uid %d, found)", uid, state, os.Geteuid())
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("a real trade from this user answered %d, want 200", res.StatusCode)
	}
}
