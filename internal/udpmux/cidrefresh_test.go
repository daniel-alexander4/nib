package udpmux

import (
	"net"
	"sync"
	"testing"
	"time"
)

// TestAKnownConnectionIDIsRefreshedAtHalfItsTTLNotPerPacket — /pending 501.
//
// `knownCID` took the write lock on EVERY inbound short-header datagram carrying a known id — the
// steady state of a live QUIC session — to move an expiry five minutes out by a few milliseconds.
// It now refreshes once the entry is past half its TTL, the rule `learn` already follows. The
// observable is the stored expiry: a write it made is a write it did not need.
func TestAKnownConnectionIDIsRefreshedAtHalfItsTTLNotPerPacket(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	start := time.Now()
	now := start
	m := newWithClock(pc, func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	defer m.Close()
	set := func(t time.Time) { mu.Lock(); now = t; mu.Unlock() }

	cid := []byte{0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8}
	m.RegisterConnectionID(cid)
	short := append([]byte{0x40}, cid...) // a short header: top bit clear, then the id
	short = append(short, 0, 0, 0)
	expiry := func() time.Time {
		m.cidMu.RLock()
		defer m.cidMu.RUnlock()
		return m.cids[string(cid)]
	}
	registered := expiry()

	// STIMULUS: the id is known, so every call below takes the refresh branch or decides not to.
	set(start.Add(time.Second))
	if known, _ := m.knownCID(short); !known {
		t.Fatal("setup: a just-registered id is not known, so nothing below is about refreshing one")
	}
	if got := expiry(); !got.Equal(registered) {
		t.Errorf("a packet one second after registration rewrote the expiry (%v → %v) — the write "+
			"lock is being taken per packet on a fresh entry", registered, got)
	}

	// Past half the TTL, a packet does refresh: an active connection keeps its id.
	late := start.Add(peerTTL/2 + time.Second)
	set(late)
	if known, _ := m.knownCID(short); !known {
		t.Fatal("an id at half its TTL is no longer known")
	}
	if got, want := expiry(), late.Add(peerTTL); !got.Equal(want) {
		t.Errorf("a packet past half the TTL left the expiry at %v, want %v — an active "+
			"connection's id would lapse under traffic", got, want)
	}
}
