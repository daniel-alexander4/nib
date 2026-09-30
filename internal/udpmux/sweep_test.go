package udpmux

import (
	"encoding/binary"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// TestFillingATableIsNotQuadratic — /pending 711 R5-2. Both expiring tables swept the whole
// map on every insert, under the write lock the read loop takes per datagram, so filling
// either was quadratic in how many entries a remote host chose to create (every QUIC Initial
// registers an id and every reply learns a destination). The bound is on SCALING, best of
// five: four times the entries must cost well under the sixteen times a per-insert sweep
// costs.
func TestFillingATableIsNotQuadratic(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	now := time.Now()
	m := newWithClock(pc, func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	defer m.Close()
	// The clock advances a microsecond per insert — so nothing expires, which is the
	// attacker's case (every entry is fresh), and sweeps still come due on schedule.
	tick := func() { mu.Lock(); now = now.Add(time.Microsecond); mu.Unlock() }

	fill := map[string]func(i int){
		"RegisterConnectionID": func(i int) {
			var id [8]byte
			binary.BigEndian.PutUint64(id[:], uint64(i))
			m.RegisterConnectionID(id[:])
		},
		"learn": func(i int) {
			m.learn(&net.UDPAddr{IP: net.IPv4(10, byte(i>>16), byte(i>>8), byte(i)), Port: 4000 + i%1000})
		},
	}
	const small = 2000
	offset := 0
	for name, insert := range fill {
		cost := func(n int) time.Duration {
			best := time.Duration(1<<63 - 1)
			for r := 0; r < 5; r++ {
				// Fresh keys every round, so each round grows the table by n.
				m.mu.Lock()
				m.peers = map[netip.AddrPort]time.Time{}
				m.mu.Unlock()
				m.cidMu.Lock()
				m.cids = map[string]time.Time{}
				m.cidMu.Unlock()
				start := time.Now()
				for i := 0; i < n; i++ {
					insert(offset + i)
					tick()
				}
				if d := time.Since(start); d < best {
					best = d
				}
				offset += n
			}
			return best
		}
		a, b := cost(small), cost(4*small)
		if b > 8*a {
			t.Errorf("%s: %d inserts took %v and %d took %v (%.1fx for 4x the entries) — an insert "+
				"is paying for the table's size again", name, small, a, 4*small, b, float64(b)/float64(a))
		}
	}
}
