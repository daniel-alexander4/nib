package p2p

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"nib/internal/sign"
)

// TestOneStallingSourceCannotHoldTheGenuinePeerOut — /pending 775, ADR-149.
//
// The TCP handshake pool is sixteen slots and its acquire blocks, so a host that kept opening
// connections and saying nothing put the genuine peer behind everything it had queued: N stalled
// connections cost about N/16 handshake timeouts (30 s each), and at 160 that is the whole arm.
// A source now holds at most `maxHandshakesPerSource` slots and the rest of what it sends is
// closed, so the genuine peer's handshake starts at once.
//
// Three times the pool, from one source, held open and silent — then a pinned dial from another
// source, which must complete in seconds where the uncapped pool makes it wait a minute and a half.
func TestOneStallingSourceCannotHoldTheGenuinePeerOut(t *testing.T) {
	certA, keyA, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	certB, keyB, err := sign.GenerateIdentity("Bob")
	if err != nil {
		t.Fatal(err)
	}
	fpA, _ := sign.Fingerprint(certA)
	fpB, _ := sign.Fingerprint(certB)

	ln, err := Listen("127.0.0.1:0", certB, keyB, fpA)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	tl, ok := ln.(*tlsListener)
	if !ok {
		t.Fatalf("setup: Listen returned %T, not the TCP listener the cap is on", ln)
	}
	// Every address this test can dial from is loopback, which the cap exempts by name — so the
	// exemption is lifted for this listener, before its accept loop starts.
	tl.capLoopback = true
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			c.Close()
		}
	}()

	// The stalling source: its own address, so it and the genuine peer are two sources.
	stall := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 2)}, Timeout: 5 * time.Second}
	const n = 3 * maxConcurrentHandshakes
	var held []net.Conn
	defer func() {
		for _, c := range held {
			c.Close()
		}
	}()
	for i := 0; i < n; i++ {
		c, derr := stall.Dial("tcp", ln.Addr().String())
		if derr != nil {
			if i == 0 {
				t.Skipf("cannot dial from a second loopback address here: %v", derr)
			}
			t.Fatalf("setup: stalled connection %d of %d did not open: %v", i+1, n, derr)
		}
		held = append(held, c)
	}

	// THE ASSERTION: the genuine peer, from another source, is not behind any of that.
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, derr := Dial(ctx, ln.Addr().String(), certA, keyA, fpB, 10*time.Second)
	waited := time.Since(start)
	if derr != nil {
		t.Fatalf("with %d silent connections open from one other source, the pinned peer could not "+
			"complete a handshake in %s: %v — that source holds the whole pool, and the genuine "+
			"peer waits about %d handshake timeouts behind it", n, waited.Round(time.Millisecond), derr,
			n/maxConcurrentHandshakes)
	}
	c.Close()
	t.Logf("the pinned peer's handshake took %s behind %d stalled connections from one source",
		waited.Round(time.Millisecond), n)

	// And the stalling source was refused, not queued: all but its share were closed by the listener.
	closed := 0
	for _, c := range held {
		_ = c.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		var b [1]byte
		if _, rerr := c.Read(b[:]); rerr != nil {
			if ne, isNet := rerr.(net.Error); isNet && ne.Timeout() {
				continue // still open: one of the slots this source is allowed
			}
			closed++
		}
	}
	if open := n - closed; open > maxHandshakesPerSource {
		t.Errorf("%d of the stalling source's %d connections are still open, want at most %d — the "+
			"rest must be closed, not left queued behind the pool", open, n, maxHandshakesPerSource)
	}
	if closed == n {
		t.Errorf("every one of the stalling source's connections was closed, so nothing was holding "+
			"a slot and the assertion above proves nothing about a held pool (cap %d)", maxHandshakesPerSource)
	}
}

// TestAHandshakeSourceIsCountedAndGivenBack — the cap's bookkeeping, at the door: a source is admitted
// up to its share, refused past it, admitted again when one of its handshakes ends, and forgotten
// when none is left.
func TestAHandshakeSourceIsCountedAndGivenBack(t *testing.T) {
	l := newListenerCore()
	src := &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 4000}
	var releases []func()
	for i := 0; i < maxHandshakesPerSource; i++ {
		release, ok := l.admit(src)
		if !ok {
			t.Fatalf("connection %d from one source was refused, below the cap of %d", i+1, maxHandshakesPerSource)
		}
		releases = append(releases, release)
	}
	if _, ok := l.admit(&net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 4001}); ok {
		t.Fatalf("connection %d from one source was admitted, past the cap", maxHandshakesPerSource+1)
	}
	if _, ok := l.admit(&net.TCPAddr{IP: net.ParseIP("203.0.113.8"), Port: 4000}); !ok {
		t.Error("another source was refused because the first was at its cap — the cap is per source")
	}
	releases[0]()
	release, ok := l.admit(src)
	if !ok {
		t.Fatal("a source that gave one slot back was still refused")
	}
	release()
	for _, r := range releases[1:] {
		r()
	}
	if n := l.sources[netip.MustParsePrefix("203.0.113.7/32")]; n != 0 {
		t.Errorf("the source still counts %d handshakes after every one ended", n)
	}
	if _, kept := l.sources[netip.MustParsePrefix("203.0.113.7/32")]; kept {
		t.Error("a source with no handshake in flight is still in the table — one entry per stranger, for the listener's life")
	}
}

// TestWhatCountsAsOneHandshakeSource — the key, and the named exemption.
func TestWhatCountsAsOneHandshakeSource(t *testing.T) {
	l := newListenerCore()
	key := func(ip string) (netip.Prefix, bool) {
		return l.handshakeSource(&net.TCPAddr{IP: net.ParseIP(ip), Port: 1})
	}
	a, capped := key("2001:db8:1:2::1")
	b, _ := key("2001:db8:1:2:ffff::9")
	if !capped || a != b {
		t.Errorf("two addresses of one IPv6 /64 are keyed %v and %v (capped=%v) — a host owns its /64, "+
			"so per-address keys hand it 2^64 sources", a, b, capped)
	}
	if c, _ := key("2001:db8:1:3::1"); c == a {
		t.Error("two different /64s share a key")
	}
	v4, capped := key("198.51.100.4")
	if other, _ := key("198.51.100.5"); !capped || v4 == other {
		t.Errorf("two IPv4 addresses share a key (%v) or are not capped (%v)", v4, capped)
	}
	if mapped, _ := key("::ffff:198.51.100.4"); mapped != v4 {
		t.Errorf("an IPv4-mapped address is keyed %v, its IPv4 form %v — one host, two shares", mapped, v4)
	}
	for _, lo := range []string{"127.0.0.1", "127.0.0.2", "::1"} {
		if _, capped := key(lo); capped {
			t.Errorf("%s is capped — loopback is exempt by name: every tier dials from it", lo)
		}
	}
	l.capLoopback = true
	if _, capped := key("127.0.0.1"); !capped {
		t.Error("with the exemption lifted, loopback is still not capped")
	}
	if _, capped := l.handshakeSource(&net.UDPAddr{IP: net.ParseIP("198.51.100.4")}); capped {
		t.Error("an address that is not a TCP address was capped")
	}
}
