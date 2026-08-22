package server

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"
)

// The IPv4 hole-punch (P05.S08b, D8 tier 4). Both peers send NAT-opening datagrams to each
// other's published tier-4 addresses at a stepping-down cadence, bounded by a per-side packet
// budget, until a mapping coincides and the initiator's QUIC Initial traverses. This file is the
// cadence and the budget — the two D16/D33 governors — as pure, testable pieces; the sender that
// drives them and the address exchange are the rest of the slice.

// D16's punch cadence (the clock table) and D33's budget, the constants owed to S08.
const (
	// punchFastEvery / punchSlowEvery: 250 ms for the first punchFastWindow, then 1 s, to the
	// connect deadline. The step-down is criterion 14 ("nothing emits at full rate for the whole
	// deadline"): at 250 ms for 300 s a single candidate would be ~1,200 packets; stepping down
	// after 30 s makes it 390.
	punchFastEvery  = 250 * time.Millisecond
	punchSlowEvery  = 1 * time.Second
	punchFastWindow = 30 * time.Second

	// punchBudgetPerSide is D33: 3,000 packets per HOP per SIDE across all candidates — LAW, not
	// tunable. A full hop of 8 candidates at the stepped cadence is 8×390 = 3,120, ~4% over; the
	// cap is hard and trims the tail of the last candidate's retries by design (D33's own words),
	// so this is authoritative and 390/candidate is the derived, not-enforced figure. Exceeding
	// it drops and reports; it never fails the ceremony.
	punchBudgetPerSide = 3000
)

// punchInterval is the delay before the next punch datagram, a PURE function of elapsed time —
// so the step-down is asserted at the boundary without waiting 30 s of wall clock (grill: there
// is no fake clock, "driven" means driving this function and the counter, not real time).
func punchInterval(elapsed time.Duration) time.Duration {
	if elapsed < punchFastWindow {
		return punchFastEvery
	}
	return punchSlowEvery
}

// punchBudget is the per-(hop, side) packet counter across all candidates (D33). One lives per
// ceremonyID (a ceremonyID IS one hop, one side), and every punch datagram spends one before it
// is sent. It drops-and-reports on exhaustion rather than failing the ceremony.
type punchBudget struct {
	mu      sync.Mutex
	spent   int
	dropped int
}

// spend reserves one packet if the budget allows, returning false (and counting a drop) when the
// cap is reached. **Checked BEFORE the send** (grill CONFIRMED-4 sharpening) so the 8th candidate
// cannot overshoot 3,000. It deliberately does NOT reset on candidate churn: a refreshed S07
// mapping is a new candidate spending the SAME 3,000 faster, which is correct.
func (b *punchBudget) spend() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.spent >= punchBudgetPerSide {
		b.dropped++
		return false
	}
	b.spent++
	return true
}

// report returns how many were sent and how many dropped over the cap — the "reports" half of
// D33's drop-and-report, for D19/S11 to surface.
func (b *punchBudget) report() (spent, dropped int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent, b.dropped
}

// ipv4Target returns the candidate's address as a *net.UDPAddr if it is a punch target — an
// IPv4 address (tier 3 mapped or tier 4 reflexive). IPv6 (tier 2) is dialled directly and needs
// no hole; a LAN candidate never reaches here (it is not DHT-signalled). Returns nil otherwise,
// so the punch spends no budget on an address that does not need it.
func ipv4Target(c candidate) *net.UDPAddr {
	ap, err := netip.ParseAddrPort(c.Addr)
	if err != nil || !ap.Addr().Unmap().Is4() {
		return nil
	}
	return net.UDPAddrFromAddrPort(ap)
}

// punchLoop emits NAT-opening datagrams to each IPv4 candidate at the stepping-down cadence,
// sharing one per-side budget, until the context ends or the budget is exhausted. Both the
// armed side and the dialing side run one (the punch is symmetric-send, D17).
//
// **`interval` is a parameter, not a call to `punchInterval` directly** (grill CONFIRMED-5): a
// test drives the REAL loop with a fast interval, so "the sender emits on a step-down cadence,
// bounded by the budget" is exercised rather than only the pure function. Production passes
// `punchInterval`.
func punchLoop(ctx context.Context, punch func(net.Addr) error, budget *punchBudget, cands <-chan candidate, interval func(time.Duration) time.Duration) {
	start := time.Now()
	var wg sync.WaitGroup
	feed := func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case c, ok := <-cands:
				if !ok {
					return
				}
				addr := ipv4Target(c)
				if addr == nil {
					continue // not an IPv4 punch target — no budget spent
				}
				wg.Add(1)
				go punchOne(ctx, punch, budget, addr, start, interval, &wg)
			}
		}
	}
	wg.Add(1)
	feed()
	wg.Wait()
}

// punchOne retransmits to one address until the context ends or the shared budget is spent.
func punchOne(ctx context.Context, punch func(net.Addr) error, budget *punchBudget, addr net.Addr, start time.Time, interval func(time.Duration) time.Duration, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		if !budget.spend() {
			return // per-side budget exhausted: drop-and-report (report() carries the count)
		}
		_ = punch(addr)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval(time.Since(start))):
		}
	}
}
