package udpmux

import (
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// TestAPanicRoutingOneDatagramCostsThatDatagramAndNotTheSocket — /pending 512.
//
// The read loop's only recover sat at the TOP of the goroutine, which is where `safe.Recover`
// belongs and is also what made a recovered panic fatal to the mux: the goroutine ended, both
// views went on blocking against a socket nobody was reading, and nothing anywhere said so.
// Calling `shut` is not the alternative it looks like — anacrolix/dht's `serveUntilClosed`
// panics on a read error while the socket is open, so shutting turns the recover back into the
// crash it exists to prevent. The per-datagram work carries its own recover instead, and this
// asserts both halves of what that buys: the datagram is COUNTED, and the socket still works
// afterwards.
//
// # How a panic is reached at all, since no datagram can cause one
//
// /pending 512 recorded that no input was found that panics in `route`/`peerKey`/`knownCID`/
// `deliver`, and reading them says why: `knownCID` refuses `len(p) < 1+n` before it indexes,
// `peerKey` type-asserts with `ok` and `(*net.UDPAddr).AddrPort` is nil-safe, and `deliver`
// sends on a channel nothing in this package ever closes (`grep -n "close(s.in)" mux.go` is
// empty). That is exactly why the defect could ship: the recover was unreachable, so its
// behaviour was never observed.
//
// The CLOCK is the seam, and it is the one this package's own `TestAPeerExpires` already
// drives: `newWithClock` is how a test supplies `now`, and `knownCID` calls it on every
// datagram. Arming it to panic puts a panic inside `route` while a real datagram is being
// routed off a real socket — the shape the recover exists for — without pretending the wire
// can produce one.
func TestAPanicRoutingOneDatagramCostsThatDatagramAndNotTheSocket(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Armed after construction, never during: `RegisterConnectionID` below reads the clock on
	// this goroutine, and a clock that panicked there would fail the test from the setup rather
	// than from the read loop.
	var armed atomic.Bool
	m := newWithClock(pc, func() time.Time {
		if armed.Load() {
			panic("udpmux test: the clock panicked while a datagram was being routed")
		}
		return time.Now()
	})
	defer m.Close()

	// A registered connection id is what makes `knownCID` reach the clock at all.
	m.UseConnectionIDs(8)
	m.RegisterConnectionID([]byte{9, 9, 9, 9, 9, 9, 9, 9})

	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	// STIMULUS: the same datagram routes before the clock is armed, so arming it is the only
	// difference between this arrival and the loss below.
	if _, err := peer.WriteTo(quicShortHeader, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if !arrives(t, m.QUIC(), quicShortHeader) {
		t.Fatal("a short header carrying a registered connection id did not reach QUIC with the " +
			"clock unarmed; nothing below could fail")
	}

	armed.Store(true)
	if _, err := peer.WriteTo(quicShortHeader, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}

	// The datagram is lost — and the loss is COUNTED, which is the whole of the diagnosability
	// half. Polled rather than slept on: the read loop is a goroutine and the write above only
	// hands the datagram to the kernel.
	deadline := time.Now().Add(2 * time.Second)
	for m.Stats().Panicked == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if got := m.Stats().Panicked; got != 1 {
		t.Fatalf("Stats().Panicked = %d after a panic routing one datagram, want 1 — "+
			"a datagram Nib dropped through its own bug left no trace anywhere", got)
	}

	// AND THE POINT: the socket is still being read. Disarm and send a real KRPC ping; it must
	// reach the DHT view, which it cannot do unless the read loop survived the panic.
	armed.Store(false)
	if _, err := peer.WriteTo(krpcPing, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if !arrives(t, m.DHT(), krpcPing) {
		t.Fatal("a datagram sent after the panic never arrived: the read loop died with it, " +
			"which is the defect — one bad datagram took the whole shared socket down")
	}

	// The panicking datagram was dropped rather than delivered to either view: nothing may act
	// on a datagram whose routing did not finish.
	if got := m.Stats().DroppedQUIC + m.Stats().DroppedDHT; got != 0 {
		t.Fatalf("a view dropped %d datagram(s); the panic path is not supposed to enqueue "+
			"anything at all", got)
	}
}
