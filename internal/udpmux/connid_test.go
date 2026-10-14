package udpmux

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// shortWith is a short-header datagram addressed to the connection id `id`.
func shortWith(id []byte) []byte {
	p := append([]byte{0x40}, id...)
	return append(p, 0xab, 0xcd, 0xef)
}

// TestAnIssuedConnectionIDIsOursHoweverLongAgoItWasIssued — /pending 669 #2, /pending 691 R7-3.
//
// The mux used to remember each id it issued for five minutes from its last match, so a spare
// id quic-go had queued at the handshake was forgotten while the humans read the safety words,
// and the peer's rotation onto it sent the rest of the transfer to the DHT. An id now proves
// itself, so its age is not a question. Both directions are asserted: what was issued is known
// whatever the clock says, and what was not issued is not known whatever it looks like.
func TestAnIssuedConnectionIDIsOursHoweverLongAgoItWasIssued(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	now := time.Now()
	m := newWithClock(pc, func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	defer m.Close()

	// Before a generator is installed the mux has been told nothing, and says so: that second
	// return is what keeps the address rule deciding.
	if known, any := m.knownCID(shortWith(make([]byte, ConnectionIDLen))); known || any {
		t.Fatalf("with no generator installed knownCID = (%v, %v), want (false, false) — the "+
			"address rule would be switched off before anything replaced it", known, any)
	}

	var ids [][]byte
	for i := 0; i < 6; i++ { // quic-go holds at most six issued at once
		id, err := m.NewConnectionID()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) != ConnectionIDLen {
			t.Fatalf("an issued id is %d bytes, want %d", len(id), ConnectionIDLen)
		}
		ids = append(ids, id)
	}

	// The pause: a month, during which none of them is matched even once.
	mu.Lock()
	now = now.Add(30 * 24 * time.Hour)
	mu.Unlock()
	for i, id := range ids {
		known, any := m.knownCID(shortWith(id))
		if !known || !any {
			t.Errorf("id %d issued a month ago and never used: knownCID = (%v, %v), want (true, true) "+
				"— a spare id the peer rotates to after a pause goes to the DHT and the session stalls",
				i, known, any)
		}
		if got := m.route(shortWith(id), nil); got != m.quic {
			t.Errorf("a short header for issued id %d was not routed to QUIC", i)
		}
	}

	// What this Mux did not issue is not its own.
	stranger := func(name string, id []byte) {
		t.Helper()
		if known, any := m.knownCID(shortWith(id)); known || !any {
			t.Errorf("%s: knownCID = (%v, %v), want (false, true) — the mux claims an id it never "+
				"issued, so a DHT datagram shaped like it is swallowed by QUIC", name, known, any)
		}
	}
	random := make([]byte, ConnectionIDLen)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	stranger("sixteen random bytes", random)
	stranger("the bytes a KRPC ping carries there", krpcPing[1:1+ConnectionIDLen])
	flipNonce := bytes.Clone(ids[0])
	flipNonce[0] ^= 1
	stranger("an issued id with one bit of its random half changed", flipNonce)
	flipTag := bytes.Clone(ids[0])
	flipTag[ConnectionIDLen-1] ^= 1
	stranger("an issued id with one bit of its tag changed", flipTag)

	other := New(mustListen(t))
	defer other.Close()
	foreign, err := other.NewConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	stranger("an id another Mux issued", foreign)

	// A datagram too short to hold an id is not one of ours, and is not read past its end.
	if known, _ := m.knownCID(shortWith(ids[0])[:ConnectionIDLen]); known {
		t.Error("a short header one byte short of holding a connection id was called ours")
	}
}

func mustListen(t *testing.T) net.PacketConn {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return pc
}

// TestATransferAfterALongPauseCrossesItsConnectionIDRotations is the same property over real
// quic-go, which is where it was a defect: nothing in this package decides WHEN a peer moves to
// a spare connection id — quic-go does, every 5,000-15,000 packets — so the reach is only
// visible with the library in the path.
//
// Two muxes on one test clock, a session, six minutes of pause with a byte every two (which
// kept the id in use fresh under the old table and left the spares to lapse), then enough data
// to force rotations. Measured against the table this replaced: the write stalled at about
// 13,000 packets with 270 datagrams in the receiver's DHT queue.
func TestATransferAfterALongPauseCrossesItsConnectionIDRotations(t *testing.T) {
	var mu sync.Mutex
	now := time.Now()
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	a, b := newWithClock(mustListen(t), clock), newWithClock(mustListen(t), clock)
	defer a.Close()
	defer b.Close()

	tr := &quic.Transport{Conn: a.QUIC(), ConnectionIDGenerator: newCIDGen(a)}
	defer tr.Close()
	srvTr := &quic.Transport{Conn: b.QUIC(), ConnectionIDGenerator: newCIDGen(b)}
	defer srvTr.Close()
	ln, err := srvTr.Listen(&tls.Config{
		Certificates: []tls.Certificate{selfSigned(t)}, NextProtos: []string{"nib-pause"},
	}, &quic.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	const pauses, size = 4, 40 << 20
	seen := make(chan struct{}, pauses)
	got := make(chan int64, 1)
	go func() {
		c, err := ln.Accept(ctx)
		if err != nil {
			got <- -1
			return
		}
		st, err := c.AcceptStream(ctx)
		if err != nil {
			got <- -1
			return
		}
		one := make([]byte, 1)
		for i := 0; i < pauses; i++ {
			if _, err := io.ReadFull(st, one); err != nil {
				got <- -1
				return
			}
			seen <- struct{}{}
		}
		n, _ := io.Copy(io.Discard, st)
		got <- n
	}()

	conn, err := tr.Dial(ctx, b.LocalAddr(), &tls.Config{
		InsecureSkipVerify: true, NextProtos: []string{"nib-pause"},
	}, &quic.Config{})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseWithError(0, "")
	st, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < pauses; i++ {
		if _, err := st.Write([]byte{1}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-seen:
		case <-ctx.Done():
			t.Fatal("the session never carried its first bytes; nothing below is about a pause")
		}
		if i < pauses-1 {
			mu.Lock()
			now = now.Add(2 * time.Minute)
			mu.Unlock()
		}
	}

	go func() {
		st.Write(make([]byte, size))
		st.Close()
	}()
	select {
	case n := <-got:
		if n != size {
			t.Fatalf("after the pause %d of %d bytes arrived (receiving mux %+v)", n, size, b.Stats())
		}
	case <-time.After(60 * time.Second):
		t.Fatalf("the transfer after a six-minute pause stalled (receiving mux %+v) — the peer "+
			"rotated to a connection id this mux no longer calls its own", b.Stats())
	}
	// STIMULUS: the transfer was long enough for the sender to rotate at all — 40 MiB is some
	// 30,000 packets against a rotation every 15,000 at most.
	if bs := b.Stats(); bs.RoutedByCID < 20000 {
		t.Fatalf("only %d datagrams were routed by connection id; the transfer was too short to "+
			"cross a rotation and proved nothing", bs.RoutedByCID)
	}
	if bs, as := b.Stats(), a.Stats(); bs.RoutedToDHT != 0 || as.RoutedToDHT != 0 || bs.RoutedByPeer != 0 {
		t.Errorf("session datagrams left the connection-id rule: receiver %+v, sender %+v", bs, as)
	}
}
