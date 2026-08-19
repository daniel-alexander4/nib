package udpmux

import (
	"bytes"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// krpcPing is a real KRPC ping query, bencoded exactly as the BitTorrent DHT
// sends one. It is a fixture and not a construction, because the whole point of
// the routing rules is what the FIRST BYTE of a real message on the wire is.
var krpcPing = []byte("d1:ad2:id20:abcdefghij0123456789e1:q4:ping1:t2:aa1:y1:qe")

// quicLongHeader is the shape of a QUIC Initial: header form bit set.
var quicLongHeader = []byte{0xc0, 0x00, 0x00, 0x00, 0x01, 0x08, 1, 2, 3, 4, 5, 6, 7, 8}

// quicShortHeader is the steady state of an established connection: header form
// clear, fixed bit set. This is byte-for-byte indistinguishable from a bencode
// dictionary by the leading-byte test, which is the point of the package.
var quicShortHeader = []byte{0x40, 9, 9, 9, 9, 9, 9, 9, 9, 0xab, 0xcd}

func newTestMux(t *testing.T) (*Mux, *net.UDPConn) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := New(pc)
	t.Cleanup(func() { m.Close() })

	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { peer.Close() })
	return m, peer
}

// arrives reads one datagram from a view, or reports that none came.
func arrives(t *testing.T, view net.PacketConn, want []byte) bool {
	t.Helper()
	view.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	defer view.SetReadDeadline(time.Time{})
	buf := make([]byte, 2048)
	n, _, err := view.ReadFrom(buf)
	if err != nil {
		return false
	}
	if !bytes.Equal(buf[:n], want) {
		t.Fatalf("view delivered %q, want %q", buf[:n], want)
	}
	return true
}

// TestTheCheapDiscriminatorIsWrong is the assertion the slice exists to leave
// behind: the leading-byte test that looks obvious collides on the steady state
// of every established connection, so nobody re-derives it and ships it.
//
// quic-go's own passthrough hook is built on exactly this arithmetic
// (wire.IsPotentialQUICPacket(b) = b&0x40 > 0), which is why that hook cannot
// carry KRPC and why this package exists.
func TestTheCheapDiscriminatorIsWrong(t *testing.T) {
	// The premise, asserted rather than asserted-about: a real KRPC message is a
	// bencode DICTIONARY, so it begins with 'd'.
	if krpcPing[0] != 'd' {
		t.Fatalf("the fixture is not a bencode dictionary: first byte %#x", krpcPing[0])
	}
	if 'd' != 0x64 {
		t.Fatal("arithmetic")
	}

	// The collision. QUIC's fixed bit is SET on a bencode 'd' — so the cheap test
	// calls a DHT ping a QUIC packet.
	if krpcPing[0]&0x40 == 0 {
		t.Fatal("premise gone: 'd' no longer has QUIC's fixed bit set — re-derive the rules")
	}
	if quicShortHeader[0]&0x40 == 0 {
		t.Fatal("the short-header fixture is not a short header")
	}
	// Stated as the thing it is: on the fixed bit, the two are the same packet.
	if (krpcPing[0]&0xc0)>>6 != (quicShortHeader[0]&0xc0)>>6 {
		t.Fatal("the collision this package exists for is gone; the mux may be simplifiable")
	}

	// And the rule that DOES separate them, in the one direction it is safe:
	// the top bit. A bencode dictionary can never set it.
	if krpcPing[0]&0x80 != 0 {
		t.Fatal("'d' has the header-form bit set — rule 1 is unsound")
	}
	if quicLongHeader[0]&0x80 == 0 {
		t.Fatal("the long-header fixture is not a long header")
	}

	// Driven, not argued: the same bytes through the real socket.
	m, peer := newTestMux(t)
	if _, err := peer.WriteTo(krpcPing, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if !arrives(t, m.DHT(), krpcPing) {
		t.Fatal("a KRPC ping did not reach the DHT view — the cheap discriminator is in the path")
	}
}

// TestALongHeaderFromAnUnknownAddressReachesQUIC — rule 1, which is the only rule
// that can speak for an address never seen before, and so the one that lets a peer
// dial IN to a socket the DHT is also using.
func TestALongHeaderFromAnUnknownAddressReachesQUIC(t *testing.T) {
	m, peer := newTestMux(t)

	// The stimulus assertion: this address really is unknown. If the peer table
	// already held it, the test would pass by rule 2 and prove nothing.
	if m.Stats().Peers != 0 {
		t.Fatal("the peer table is not empty at the start; this test would pass by rule 2")
	}

	if _, err := peer.WriteTo(quicLongHeader, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if !arrives(t, m.QUIC(), quicLongHeader) {
		t.Fatal("a long-header packet from an unknown address did not reach the QUIC view")
	}
	if got := m.Stats().RoutedLongHeader; got != 1 {
		t.Fatalf("RoutedLongHeader = %d, want 1 — it arrived by some other rule", got)
	}
}

// TestAShortHeaderReachesQUICOnlyAfterWeHaveWrittenThere is a TRANSITION check,
// not a state check: it asserts the short header goes to the DHT BEFORE the peer
// is learned, and to QUIC after. Asserting only the second half would pass against
// a mux that sent everything to QUIC.
func TestAShortHeaderReachesQUICOnlyAfterWeHaveWrittenThere(t *testing.T) {
	m, peer := newTestMux(t)

	// Before: unknown address, no top bit — the DHT gets it.
	if _, err := peer.WriteTo(quicShortHeader, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if !arrives(t, m.DHT(), quicShortHeader) {
		t.Fatal("before learning, a short header should reach the DHT view")
	}

	// The transition: one outbound QUIC write to that address.
	if _, err := m.QUIC().WriteTo([]byte("x"), peer.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if got := m.Stats().Learned; got != 1 {
		t.Fatalf("Learned = %d, want 1 — the write taught the router nothing", got)
	}

	// After: the same bytes from the same address now reach QUIC.
	if _, err := peer.WriteTo(quicShortHeader, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if !arrives(t, m.QUIC(), quicShortHeader) {
		t.Fatal("after learning, a short header should reach the QUIC view")
	}
	if got := m.Stats().RoutedByPeer; got != 1 {
		t.Fatalf("RoutedByPeer = %d, want 1", got)
	}
}

// TestAnInboundLongHeaderTeachesNothing guards the anti-flood property the
// package documents: the peer table grows from OUR writes, never from theirs.
// Without this, forged long headers from spoofed addresses grow it without bound.
func TestAnInboundLongHeaderTeachesNothing(t *testing.T) {
	m, peer := newTestMux(t)
	for i := 0; i < 50; i++ {
		if _, err := peer.WriteTo(quicLongHeader, m.LocalAddr()); err != nil {
			t.Fatal(err)
		}
	}
	// Stimulus first: the datagrams really did arrive and route.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && m.Stats().RoutedLongHeader == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if m.Stats().RoutedLongHeader == 0 {
		t.Fatal("nothing arrived; the test could not have failed")
	}
	if got := m.Stats().Peers; got != 0 {
		t.Fatalf("Peers = %d after %d inbound long headers, want 0 — remote input is growing the table",
			got, m.Stats().RoutedLongHeader)
	}
}

// TestAPeerExpires — an address stops being a QUIC peer once it has been idle for
// peerTTL, so it can be an ordinary DHT node again.
func TestAPeerExpires(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Constructed with the clock rather than having it reassigned afterwards: the
	// read loop calls now() on every datagram, so assigning the field on a live Mux
	// is a data race that happens to be invisible while no datagram arrives.
	var mu sync.Mutex
	now := time.Now()
	m := newWithClock(pc, func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	defer m.Close()
	set := func(t time.Time) { mu.Lock(); now = t; mu.Unlock() }

	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 65000}
	other := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 65001}
	m.learn(addr)
	if !m.isQUICPeer(addr) {
		t.Fatal("a just-learned address is not a peer; the rest of the test is vacuous")
	}

	set(now.Add(peerTTL + time.Second))
	if m.isQUICPeer(addr) {
		t.Fatal("an address idle past peerTTL is still routed to QUIC")
	}
	// And the sweep really removes it, rather than leaving it to grow the map.
	m.learn(other)
	if got := m.Stats().Peers; got != 1 {
		t.Fatalf("Peers = %d after the sweep, want 1 (only the fresh one)", got)
	}
	if got := m.Stats().Expired; got != 1 {
		t.Fatalf("Expired = %d, want 1", got)
	}

	// A write refreshes: the same address written again is a peer again.
	m.learn(addr)
	if !m.isQUICPeer(addr) {
		t.Fatal("a re-written address did not become a peer again")
	}
}

// TestClosingOneViewLeavesTheOtherRunning is the reason a shim exists at all:
// quic.Transport.Close and dht.Server.Close each close the conn they were given,
// and neither may take the other's transport down with it.
func TestClosingOneViewLeavesTheOtherRunning(t *testing.T) {
	m, peer := newTestMux(t)

	// Stimulus: both views work first.
	if _, err := peer.WriteTo(krpcPing, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if !arrives(t, m.DHT(), krpcPing) {
		t.Fatal("the DHT view does not work before the close; nothing below could fail")
	}

	if err := m.QUIC().Close(); err != nil {
		t.Fatal(err)
	}

	// The socket is still open and the other view still delivers.
	if _, err := peer.WriteTo(krpcPing, m.LocalAddr()); err != nil {
		t.Fatalf("the shared socket died with one view: %v", err)
	}
	if !arrives(t, m.DHT(), krpcPing) {
		t.Fatal("closing the QUIC view took the DHT view down with it")
	}

	// The closed view is closed, and says so as net.ErrClosed.
	buf := make([]byte, 64)
	if _, _, err := m.QUIC().ReadFrom(buf); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed view read err = %v, want net.ErrClosed", err)
	}
	if _, err := m.QUIC().WriteTo([]byte("x"), peer.LocalAddr()); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed view write err = %v, want net.ErrClosed", err)
	}
}

// TestADeadlineExpiresAndUnexpires guards a dependency read out of quic-go rather
// than guessed: Transport.Close on a USER-SUPPLIED conn does not close it — it
// calls SetReadDeadline(time.Now()) to unblock its read loop and then resets the
// deadline to zero (transport.go:460). Two properties follow, and both are load-
// bearing: the error must satisfy net.Error with Temporary() true, or quic-go's
// listen loop treats it as fatal rather than as the shutdown signal it is; and the
// view must be readable again afterwards, or every transport close would leave the
// shared socket half dead.
func TestADeadlineExpiresAndUnexpires(t *testing.T) {
	m, peer := newTestMux(t)

	m.QUIC().SetReadDeadline(time.Now().Add(-time.Second))
	buf := make([]byte, 64)
	_, _, err := m.QUIC().ReadFrom(buf)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read past the deadline returned %v, want os.ErrDeadlineExceeded", err)
	}
	ne, ok := err.(net.Error)
	if !ok || !ne.Timeout() {
		t.Fatalf("the deadline error is not a net.Error timeout: %v", err)
	}
	//nolint:staticcheck // Temporary() is deprecated, and quic-go's listen loop reads it anyway.
	if !ne.Temporary() {
		t.Fatal("the deadline error is not Temporary(); quic-go's listen loop would treat a " +
			"transport close as a fatal read error instead of a shutdown")
	}

	// Un-expire, exactly as quic-go's deferred reset does — and assert it with a
	// read that must BLOCK and then succeed. The obvious shape (reset, send, then
	// call the arrives helper) proves nothing: arrives sets a deadline of its own,
	// and setting a FUTURE deadline recreates the channel too, so the helper would
	// paper over a reset-to-zero that did nothing. This was green against a set()
	// with the zero-time branch deleted until it was probed.
	m.QUIC().SetReadDeadline(time.Time{})
	read := make(chan error, 1)
	go func() {
		buf := make([]byte, 2048)
		n, _, err := m.QUIC().ReadFrom(buf)
		if err == nil && !bytes.Equal(buf[:n], quicLongHeader) {
			err = errors.New("wrong datagram")
		}
		read <- err
	}()
	time.Sleep(20 * time.Millisecond) // let the read block on the (reset) deadline
	if _, err := peer.WriteTo(quicLongHeader, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-read:
		if err != nil {
			t.Fatalf("the view never recovered from an expired deadline: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a read after the deadline was reset to zero never returned")
	}
}

// TestAFullQueueDropsRatherThanBlocking. Driven at the queue rather than through
// the kernel on purpose: sending enough datagrams to overflow a socket buffer
// would be testing the kernel's drop policy, not this package's. What is asserted
// here is that a consumer which has stopped reading loses ITS OWN datagrams and
// does not stall the read loop — because on this socket a stalled DHT lookup
// would otherwise silently kill a live session.
func TestAFullQueueDropsRatherThanBlocking(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := New(pc)
	defer m.Close()

	done := make(chan struct{})
	go func() {
		for i := 0; i < queueDepth+10; i++ {
			m.dht.deliver(datagram{b: []byte("x")})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("delivery blocked on a full queue — one stalled consumer can stall the socket")
	}
	if got := m.Stats().DroppedDHT; got != 10 {
		t.Fatalf("DroppedDHT = %d, want 10", got)
	}
	if got := m.Stats().DroppedQUIC; got != 0 {
		t.Fatalf("the other side dropped %d — the queues are not independent", got)
	}
}
