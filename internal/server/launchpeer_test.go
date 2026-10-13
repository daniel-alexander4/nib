package server

import (
	"net/http"
	"net/netip"
	"os"
	"testing"
)

// `/pending 701` (ADR-157): the launch key is in the browser's command line until the page trades it,
// and every user of the machine can read a command line. The trade asks the kernel who owns the
// connection it arrived on.

// tradeAs trades key at ts with the kernel's answer replaced by (uid, state).
func tradeAs(t *testing.T, srv *Server, url, key string, uid uint32, state peerState) int {
	t.Helper()
	srv.launchPeerOwner = func(_, _ netip.AddrPort) (uint32, peerState) { return uid, state }
	req, _ := http.NewRequest(http.MethodPost, url+"/api/launch", nil)
	req.Header.Set(headerLaunch, key)
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

// TestALaunchKeyOfferedByAnotherUserIsRefusedUnspent: the thief is refused, and the key is still
// good for the window it was minted for — a refusal that spent it would leave the real window on
// *Open Nib again*, which is what the thief's trade did before.
func TestALaunchKeyOfferedByAnotherUserIsRefusedUnspent(t *testing.T) {
	ts, srv := startServerWith(t)
	me := uint32(os.Geteuid())
	other := me + 1
	if other == 0 {
		other = 1
	}
	key := srv.MintLaunchKey()
	if code := tradeAs(t, srv, ts.URL, key, other, peerFound); code != http.StatusForbidden {
		t.Fatalf("a key offered on another user's connection traded with %d, want 403", code)
	}
	if code := tradeAs(t, srv, ts.URL, key, me, peerFound); code != http.StatusOK {
		t.Fatalf("after another user's refused attempt the window's own trade answered %d, want 200: the refusal spent the key", code)
	}
}

// TestOnlyANamedOtherUserIsRefused: every answer but "another user owns it" trades as it always
// did. A Nib that does not open is worse than the gap, so a doubt refuses nothing.
func TestOnlyANamedOtherUserIsRefused(t *testing.T) {
	ts, srv := startServerWith(t)
	me := uint32(os.Geteuid())
	for _, c := range []struct {
		name  string
		uid   uint32
		state peerState
	}{
		{"this user", me, peerFound},
		{"root", 0, peerFound},
		{"no such socket on this machine", 0, peerGone},
		{"the kernel will not say", 0, peerUnknown},
		// An answer that is not "found" names nobody, whatever number rides with it.
		{"unknown with a stray number", me + 7, peerUnknown},
		{"gone with a stray number", me + 7, peerGone},
	} {
		if code := tradeAs(t, srv, ts.URL, srv.MintLaunchKey(), c.uid, c.state); code != http.StatusOK {
			t.Errorf("%s: the trade answered %d, want 200", c.name, code)
		}
	}
}

// TestTheTradeAsksAboutTheConnectionItArrivedOn: the question put to the kernel names this
// request's own two ends — the server's address as the local one, the caller's as the peer.
func TestTheTradeAsksAboutTheConnectionItArrivedOn(t *testing.T) {
	ts, srv := startServerWith(t)
	var local, peer netip.AddrPort
	srv.launchPeerOwner = func(l, p netip.AddrPort) (uint32, peerState) {
		local, peer = l, p
		return 0, peerUnknown
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", nil)
	req.Header.Set(headerLaunch, srv.MintLaunchKey())
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if want := netip.MustParseAddrPort(ts.Listener.Addr().String()); local != want {
		t.Errorf("local end asked about = %v, want the listener's %v", local, want)
	}
	if !peer.IsValid() || !peer.Addr().IsLoopback() || peer == local {
		t.Errorf("peer end asked about = %v, want the caller's loopback address", peer)
	}
}
