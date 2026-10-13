package udpmux

import (
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"nib/internal/scaling"
)

// TestFillingATableIsNotQuadratic — /pending 711 R5-2. The expiring peer table swept the whole
// map on every insert, under the write lock the read loop takes per datagram, so filling
// it was quadratic in how many entries a remote host chose to create (every reply learns a
// destination). The connection-id table that shared the defect is gone: an id is recognised by
// its own bytes and nothing is stored (connid_test.go). The bound is on what an insert costs
// against the table it lands in: a batch into a table sixteen times fuller must cost nowhere
// near the sixteen times a per-insert sweep makes it.
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

	// Each table: insert is the door under test; hold empties the table and fills it with held
	// entries directly, untimed and sweep-free, so even a regressed insert pays nothing to set up.
	type table struct {
		insert func(i int)
		hold   func(held int)
	}
	fill := map[string]table{
		"learn": {
			insert: func(i int) {
				m.learn(&net.UDPAddr{IP: net.IPv4(10, byte(i>>16), byte(i>>8), byte(i)), Port: 4000 + i%1000})
			},
			hold: func(held int) {
				m.mu.Lock()
				m.peers = make(map[netip.AddrPort]time.Time, held)
				mu.Lock()
				exp := now.Add(peerTTL)
				mu.Unlock()
				for i := 0; i < held; i++ {
					m.peers[netip.AddrPortFrom(netip.AddrFrom4([4]byte{11, byte(i >> 16), byte(i >> 8), byte(i)}), 5000)] = exp
				}
				m.mu.Unlock()
			},
		},
	}
	// The batch's cost against the table it lands in, not a fill's cost against its size: a healthy
	// fill is itself superlinear once the table outgrows the cache, ×3.9-×9.9 for 4x the entries at
	// load ~20 (measured), so the ×8 a 4× fill was held to sat inside a healthy table's own spread —
	// and best-of-five per size, one size after the other, failed 2 of 8 runs against good code at
	// that load (/pending 799). A batch into a fuller table pays the cache once and a sweep per
	// insert sixteen times; `scaling` interleaves the two and brackets the emptier one.
	const batch, emptier, fuller = 5000, 5000, 80000
	offset := 0
	for name, tb := range fill {
		cost := func(held int) func() time.Duration {
			return func() time.Duration {
				tb.hold(held)
				base := offset
				offset += batch
				return scaling.TimeOnce(func() {
					for i := 0; i < batch; i++ {
						tb.insert(base + i)
						tick()
					}
				})
			}
		}
		scaling.WithinFactor(t, name+": a batch into a table 16x fuller (an insert paying for the table's size again)",
			5, cost(emptier), cost(fuller))
	}
}
