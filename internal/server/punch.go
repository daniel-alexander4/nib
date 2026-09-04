package server

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"nib/internal/safe"
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
	//
	// **HOP is the unit, and until P08.S05h the counter was keyed by CEREMONY.** D33 says so by
	// amendment rather than by original wording — *"Total punch budget = 3,000 packets per
	// ~~ceremony~~ HOP"* — and it gives the reason the struck form was struck: *"a per-ceremony
	// budget was exhausted inside the first hop … in a 31-hop ceremony hops 2–31 would get zero
	// packets."* The code implemented the struck form, so every hop after the first drew on what
	// hop 1 had already spent, and so did every delivery leg. Two guards already sat on the
	// neighbouring axes — both loops of one hop share a budget, two ceremonies do not — and
	// neither could see this one, which is why the third now exists beside them.
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

// punchBudget is the per-(hop, side) packet counter across all candidates (D33). Every punch
// datagram spends one before it is sent, and it drops-and-reports on exhaustion rather than
// failing the ceremony.
//
// # One per (hop, side) means one per MACHINE per ceremony, not one per ceremonyID
//
// This doc used to say "one lives per ceremonyID (a ceremonyID IS one hop, one side)" and the
// code built one inline at each of the two punch call sites. **A ceremonyID is not a side.** The
// armed path and the dialing path hold DIFFERENT `ceremonyID` values with different sockets, and
// P05.S09's symmetric racing has one machine running both for the same hop — `glare.go` opens by
// saying so, and `punchLoop`'s own doc says the two "share one per-side budget". They shared
// nothing: each got its own 3,000, so a side emitted **6,000 against D33's law figure of 3,000**,
// silently, with two comments asserting otherwise.
//
// So the budget is keyed by `(ceremony id, hop)` and held by the Server, which is the only thing
// on this machine that outlives both `ceremonyID`s.
//
// **That sentence used to end "a side is a machine in a proceeding; that is what `(hop, side)`
// names", and the conflation in it is how the hop axis stayed invisible for two phases**: a
// proceeding is a CEREMONY, so a key naming the proceeding names the ceremony and not the hop,
// while the sentence claimed it named `(hop, side)`. A side is a machine in a HOP.
//
// **No harness run charges this budget, and that is stated rather than left to be
// discovered.** Only `sourceDHT` candidates are teed to the punch loop —
// `feedCeremonyRace`'s merge sends the fixed `cands` to `in` alone — and `ipv4Target` then
// requires an IPv4 address. Every candidate at tier 4 is LAN or typed, so the cap, the drop
// and the report are exercised at tier 1 and nowhere else. A tier-4 clause over this
// counter could only ever report pass.
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
// D33's drop-and-report.
//
// **Its doc said "for D19/S11 to surface" and S11 shipped without wiring it**, so until P07.S09b
// the only callers were tests: D33's drop-and-report had a drop and no report. `diagnose` is the
// reader now, which is where this sentence always said it was going.
func (b *punchBudget) report() (spent, dropped int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent, b.dropped
}

// punchBudgetFor returns the one budget this machine spends on that (ceremony, HOP), creating it
// on first use. Both punch loops of one hop reach the same counter through it; two hops of one
// ceremony reach different ones, which is D33's unit.
//
// **Keyed on the pair, and the hop half is what P08.S05h added.** Keying on the ceremony alone
// implemented the form D33 struck, and the failure is silent by construction because dropping over
// the cap is the designed behaviour: hop 2 of a nine-party ceremony simply punched less, then not
// at all, and nothing anywhere said the ceiling had been reached for the proceeding rather than
// for the hop. `hopScoped(c, cer.hop)` filters the candidate stream by hop eight lines from where
// the budget was taken by ceremony, so the right unit was already in the file beside the wrong one.
//
// An empty id is a punch outside any ceremony — no proceeding to share a budget within, so it
// gets its own rather than joining a process-wide pool that would let unrelated work starve it.
func (s *Server) punchBudgetFor(id string, hop int) *punchBudget {
	if id == "" {
		return &punchBudget{}
	}
	// **Injective on `(id, hop)` whatever `id` contains**, which is the property that matters and
	// is stronger than the one this comment first gave. It said a ceremony id is 32 hex characters
	// so "#" cannot occur in it — true of an id the product minted, and `ParseInvitation` does not
	// run `ValidID`, so an id reaching here is not guaranteed to be either. The real argument needs
	// no assumption about `id`: `strconv.Itoa` emits only digits and `-`, so a "#" in the composed
	// key can only be the separator, and equal keys force equal hops and then equal ids.
	key := id + "#" + strconv.Itoa(hop)
	s.punchMu.Lock()
	defer s.punchMu.Unlock()
	if s.punchBudgets == nil {
		s.punchBudgets = map[string]*punchBudget{}
	}
	b, ok := s.punchBudgets[key]
	if !ok {
		b = &punchBudget{}
		s.punchBudgets[key] = b
	}
	return b
}

// dropPunchBudgets forgets every hop's budget for one ceremony, and returns how many it dropped.
//
// **The map had no `delete` at all until now** (`/pending 312`): one `*punchBudget` per hop the
// process ever saw, surviving every disarm and living for the process lifetime. Since P08.S05h the
// key is `(ceremony, hop)`, so a nine-party ceremony leaves eight entries plus one per delivery leg
// rather than one — the leak grows per hop, not per proceeding.
//
// **`ceremonyID.close()` is the wrong door and this one is right, which is the whole finding.** A
// machine holds TWO `ceremonyID`s for one hop — the armed one and the dialling one — and
// `server.go`'s own comment records why the map was lifted onto the `Server` in the first place:
// *"before this each built its own and a side emitted twice the law figure."* Deleting on the first
// `close()` lets the second recreate a fresh budget at zero and re-emit the full D33 figure, which
// is the exact defect the map exists to prevent. Refcounting the pair would be a second lifecycle
// mechanism beside the one the close-out door already is.
//
// **Why it is safe HERE and nowhere earlier.** Its only caller is `closeOutCeremony`, which runs on
// a proceeding the sweep has decided is over on this machine, and which drops that ceremony's
// **pins** in the same breath. A budget reset can at worst let a later punch spend the figure
// again; dropping the pins already makes that later punch impossible, so this is strictly the less
// aggressive of the two things the door does.
//
// **Split on the FIRST separator, and `punchBudgetFor`'s own argument is what licenses it**: the
// key is `id + "#" + strconv.Itoa(hop)`, and since `Itoa` emits only digits and `-`, a "#" in the
// composed key can only be the separator. So the text before the first "#" is exactly the ceremony
// id, whatever that id contains — no assumption that ids are hex, which `ParseInvitation` does not
// guarantee.
func (s *Server) dropPunchBudgets(id string) int {
	if id == "" {
		return 0 // the no-ceremony budget belongs to no proceeding and is nobody's to drop
	}
	s.punchMu.Lock()
	defer s.punchMu.Unlock()
	dropped := 0
	for key := range s.punchBudgets {
		sep := strings.IndexByte(key, '#')
		if sep < 0 || key[:sep] != id {
			continue
		}
		delete(s.punchBudgets, key)
		dropped++
	}
	return dropped
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
	defer safe.Recover("punch loop")
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
	defer safe.Recover("punch one")
	defer wg.Done()
	for {
		if !budget.spend() {
			return // per-side budget exhausted: drop-and-report (report() carries the count)
		}
		// A failed punch is discarded — including the net.ErrClosed a write can get when it
		// races the shared socket closing at teardown (diff-grill F1, benign: close() shuts the
		// DHT reader before the socket, so this writer never hits the mux-closed panic).
		_ = punch(addr)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval(time.Since(start))):
		}
	}
}
