package server

import (
	"net/http/httptest"
	"testing"
)

// /pending 19 — a typed address on a private or CGNAT range must reach the dialer.
//
// **This guards an invariant, not a bug.** The item was filed as "a VPN address can never
// be a candidate, and the README recommends one", and reading settled that the opposite is
// true: `peerAddresses` returns a typed address verbatim (lan.go) without consulting
// `addrscope`, so the VPN path works today. What was missing is any assertion that it
// keeps working.
//
// README.md tells the user, for exactly the case where port-forwarding cannot work:
// "Share a private network you both already trust — a VPN such as Tailscale or WireGuard
// — and bind / dial the address it hands you", and "if the receiver is behind CGNAT ...
// use the VPN path". That is a product promise resting on the absence of a scope check on
// one line, and nothing tested it: the named search
// `grep -rn 'sourceTyped' --include=*_test.go internal/` returns three sites and every one
// uses a PUBLIC documentation range (198.51.100.x, 203.0.113.x). So the reserved-range
// tables could grow a Target() call on this path and every test would stay green while
// Tailscale and WireGuard stopped working.
//
// The addresses below are chosen to be the ones a scope check would refuse first:
// 100.64.0.0/10 is the CGNAT range Tailscale hands out, and 10.0.0.0/8 is what a
// WireGuard peer typically gets.
func TestATypedPrivateAddressReachesTheDialer(t *testing.T) {
	// No vault and no HTTP client: peerAddresses takes the typed branch before it touches
	// either, and standing up an authenticated server would make the test depend on
	// machinery this path does not use.
	srv := New(nil, nil, t.TempDir(), "test")
	for _, tc := range []struct {
		name, addr, why string
	}{
		{"tailscale CGNAT", "100.64.0.1:9", "100.64.0.0/10 is the range Tailscale assigns, and the README names CGNAT as the case the VPN path exists for"},
		{"wireguard private", "10.0.0.5:9", "10.0.0.0/8 is an ordinary WireGuard peer address"},
		{"private class C", "192.168.1.50:9", "the commonest LAN range a user would type"},
		{"link-local v6 zoned", "[fe80::1%eth0]:9", "a link-local peer typed by hand"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			cands, ok := srv.peerAddresses(w, nil, tc.addr, "tcp", nil)
			if !ok {
				t.Fatalf("a typed %s address was refused with %d — %s. The README recommends this path and the refusal would make that advice false",
					tc.name, w.Code, tc.why)
			}
			if len(cands) != 1 || cands[0].Addr != tc.addr {
				t.Fatalf("typed %q produced %+v, want exactly one candidate carrying that address verbatim", tc.addr, cands)
			}
			if cands[0].Source != sourceTyped {
				t.Errorf("typed %q came back with source %v, want sourceTyped — a typed address that loses its source is subject to whatever rules the other sources carry", tc.addr, cands[0].Source)
			}
		})
	}
}

// The other half, and it is what stops the test above from being satisfied by a function
// that accepts everything: the transport is still validated, so a typo is refused with a
// 400 before any dial. Without this, "reaches the dialer" could be met by deleting every
// check on the path.
func TestATypedAddressStillHasItsTransportChecked(t *testing.T) {
	srv := New(nil, nil, t.TempDir(), "test")
	w := httptest.NewRecorder()
	if _, ok := srv.peerAddresses(w, nil, "100.64.0.1:9", "carrier-pigeon", nil); ok {
		t.Fatal("a typed address with an unknown transport was accepted — the scope check is absent by design here, so the transport check is the only thing left refusing a typo")
	}
	if w.Code != 400 {
		t.Errorf("bad transport answered %d, want 400", w.Code)
	}
}
