package p2p

import (
	"net"
	"testing"
	"time"
)

// v6LoopbackAvailable reports whether this host can carry IPv6 on loopback at all.
//
// A skip here is narrow and deliberate: it distinguishes "this machine has no IPv6" from "the
// bind is not dual-stack", which is the whole distinction the tests below exist to make. A
// test that simply failed on a v6-less host would be asserting the machine's network stack.
func v6LoopbackAvailable(t *testing.T) bool {
	t.Helper()
	pc, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		return false
	}
	pc.Close()
	return true
}

// TestTheWildcardBindIsDualStack pins as a PROPERTY what the plan carried as a wrong comment.
//
// P05.S05's scope said the arm's default bind is `0.0.0.0:0` and therefore "v4-only, so tier 2
// cannot work today". Measured 2026-08-22, that is false: Go rewrites a wildcard *listen* to
// AF_INET6 with IPV6_V6ONLY=0 wherever `supportsIPv4map()` holds, which is every platform Nib
// targets — `net.ListenPacket("udp", "0.0.0.0:0")` returns a socket whose `LocalAddr()` reads
// `[::]:port` and which receives datagrams sent to `[::1]:port`.
//
// **The reason this is a test and not a comment** is that the belief survived three sessions
// and a written plan scope precisely because nothing in the tree contradicted it. It is also
// load-bearing rather than trivia: the whole IPv6 tier assumes one socket answers both
// families, and the platforms where it does not (OpenBSD, DragonFly, where the v4-map probe is
// truncated) would otherwise degrade silently into a v4-only ladder with no test failing.
//
// It asserts REACHABILITY, not the address string. `LocalAddr()` reading `[::]` is what the
// kernel chose to call the socket; whether a v6 peer can actually reach it is the property the
// tier needs, and the two are not the same claim.
func TestTheWildcardBindIsDualStack(t *testing.T) {
	if !v6LoopbackAvailable(t) {
		t.Skip("this host has no IPv6 loopback, so dual-stack is not a question it can answer")
	}
	for _, bind := range []string{"0.0.0.0:0", "[::]:0"} {
		t.Run(bind, func(t *testing.T) {
			pc, err := net.ListenPacket("udp", bind)
			if err != nil {
				t.Fatalf("binding %q failed: %v", bind, err)
			}
			defer pc.Close()

			_, port, err := net.SplitHostPort(pc.LocalAddr().String())
			if err != nil {
				t.Fatal(err)
			}
			// SETUP: a v6 client must be able to exist, or the write below proves nothing.
			c, err := net.Dial("udp6", net.JoinHostPort("::1", port))
			if err != nil {
				t.Fatalf("setup: could not open a v6 client to [::1]:%s: %v", port, err)
			}
			defer c.Close()

			want := "over v6"
			if _, err := c.Write([]byte(want)); err != nil {
				t.Fatalf("writing to [::1]:%s failed: %v", port, err)
			}
			pc.SetReadDeadline(time.Now().Add(3 * time.Second))
			buf := make([]byte, 64)
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				t.Fatalf("a socket bound %q did not receive a datagram sent to [::1]:%s (%v). "+
					"It is not dual-stack on this platform, so D8's tier 2 cannot work here — "+
					"and every other tier would keep passing, which is why this is a test",
					bind, port, err)
			}
			if got := string(buf[:n]); got != want {
				t.Errorf("received %q from %s, want %q", got, from, want)
			}
		})
	}
}

// TestASharedEndpointBoundWildcardAnswersOverIPv6 is the same property through the door the
// ceremony actually uses, because `net.ListenPacket` being dual-stack says nothing about what
// `NewSharedEndpoint` does with the string it is handed.
func TestASharedEndpointBoundWildcardAnswersOverIPv6(t *testing.T) {
	if !v6LoopbackAvailable(t) {
		t.Skip("this host has no IPv6 loopback, so dual-stack is not a question it can answer")
	}
	end, err := NewSharedEndpoint("0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer end.Close()

	_, port, err := net.SplitHostPort(end.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.Dial("udp6", net.JoinHostPort("::1", port))
	if err != nil {
		t.Fatalf("setup: could not open a v6 client to the endpoint: %v", err)
	}
	defer c.Close()

	before := end.Stats()
	// A short header from an address the mux has never seen routes to the DHT view.
	if _, err := c.Write([]byte("d1:q4:ping1:y1:qe")); err != nil {
		t.Fatalf("writing to the endpoint over v6 failed: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for end.Stats().RoutedToDHT <= before.RoutedToDHT && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if end.Stats().RoutedToDHT <= before.RoutedToDHT {
		t.Errorf("a datagram sent over IPv6 to the shared endpoint at [::1]:%s never reached "+
			"the DHT view (RoutedToDHT %d); the ceremony's own socket is not answering over "+
			"IPv6, whatever a bare net.ListenPacket does", port, before.RoutedToDHT)
	}
}

// TestLocalWildcardForPicksTheRemotesFamily is this function's FIRST test of any kind.
//
// It is the only family-selecting function in the tree (`QUICDial` is its one caller), and
// until P05.S05 nothing could reach its v6 branch in production, because nothing ever produced
// a v6 candidate to dial. Now that a published record can carry one, the branch is live.
//
// The `nil` case is the one worth pinning: `ResolveUDPAddr` leaves `IP` nil for a bind like
// ":443", and that falls to the v4 wildcard. Correct — a v4 wildcard is dual-stack per the
// tests above — but it is a silent default, and a silent default is worth a written assertion.
func TestLocalWildcardForPicksTheRemotesFamily(t *testing.T) {
	for _, tc := range []struct {
		name, remote, want string
	}{
		{"ipv4", "203.0.113.5", "0.0.0.0:0"},
		{"ipv6", "2606:4700:4700::1111", "[::]:0"},
		{"ipv6 loopback", "::1", "[::]:0"},
		{"v4-mapped is v4", "::ffff:203.0.113.5", "0.0.0.0:0"},
		{"no ip at all", "", "0.0.0.0:0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ua := &net.UDPAddr{Port: 443}
			if tc.remote != "" {
				ua.IP = net.ParseIP(tc.remote)
				// SETUP: a typo in the fixture would parse to nil and silently exercise the
				// "no ip at all" row instead of the row it is named for.
				if ua.IP == nil {
					t.Fatalf("setup: %q did not parse as an IP", tc.remote)
				}
			}
			if got := localWildcardFor(ua); got != tc.want {
				t.Errorf("localWildcardFor(%q) = %q, want %q — a v6 peer dialled from a v4 "+
					"socket cannot be reached at all", tc.remote, got, tc.want)
			}
		})
	}
}
