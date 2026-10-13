package server

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The cadence steps down at the 30 s boundary — driven on the pure function, not the wall clock
// (the whole point: there is no fake clock, so this is how "criterion 14, driven" is testable).
func TestPunchIntervalStepsDownAt30s(t *testing.T) {
	for _, tc := range []struct {
		elapsed time.Duration
		want    time.Duration
	}{
		{0, punchFastEvery},
		{29 * time.Second, punchFastEvery},
		{30 * time.Second, punchSlowEvery},         // the boundary: fast BELOW 30s, slow AT 30s
		{29999 * time.Millisecond, punchFastEvery}, // just under
		{5 * time.Minute, punchSlowEvery},
	} {
		if got := punchInterval(tc.elapsed); got != tc.want {
			t.Errorf("punchInterval(%v) = %v, want %v", tc.elapsed, got, tc.want)
		}
	}
}

// The full-cadence packet count per candidate per side is 390 (120 fast + 270 slow) — the figure
// D33's 3,000/side budget is sized against. Asserted by simulating the cadence over 300s.
func TestOneCandidateIsAbout390Packets(t *testing.T) {
	const deadline = 300 * time.Second
	var elapsed time.Duration
	n := 0
	for elapsed < deadline {
		n++
		elapsed += punchInterval(elapsed)
	}
	// 30s/250ms = 120 in the fast window, 270s/1s = 270 slow → 390.
	if n < 388 || n > 392 {
		t.Errorf("one candidate emits %d packets over the cadence, want ~390 (D33's per-candidate figure)", n)
	}
}

// The budget is a hard per-side cap, checked before the send, drop-and-report, no reset.
func TestPunchBudgetIsAHardPerSideCap(t *testing.T) {
	b := &punchBudget{}
	sent := 0
	for i := 0; i < punchBudgetPerSide+500; i++ {
		if b.spend() {
			sent++
		}
	}
	if sent != punchBudgetPerSide {
		t.Errorf("budget allowed %d sends, want exactly the cap %d", sent, punchBudgetPerSide)
	}
	s, dropped := b.report()
	if s != punchBudgetPerSide {
		t.Errorf("report says %d spent, want %d", s, punchBudgetPerSide)
	}
	if dropped != 500 {
		t.Errorf("report says %d dropped, want 500 (the overflow)", dropped)
	}
}

// 8 candidates at 390 each is 3,120 — ~4% over 3,000 — and the cap trims the tail rather than
// letting the last candidate finish. Drives D33's "the ~4% is the mechanism working" claim.
func TestAFullHopTrimsTheTailAtTheCap(t *testing.T) {
	b := &punchBudget{}
	const perCandidate = 390
	const candidates = 8
	allowed := 0
	for c := 0; c < candidates; c++ {
		for i := 0; i < perCandidate; i++ {
			if b.spend() {
				allowed++
			}
		}
	}
	if allowed != punchBudgetPerSide {
		t.Errorf("a full 8×390 hop was allowed %d packets, want the cap %d (the ~120-packet tail trimmed)", allowed, punchBudgetPerSide)
	}
	if want := candidates*perCandidate - punchBudgetPerSide; want != 120 {
		t.Fatalf("arithmetic check: 8×390−3000 should be 120, got %d", want)
	}
}

// The sender emits to each IPv4 candidate, shares one budget, stops at the cap, and skips
// non-IPv4 targets — driven on the REAL loop with a fast interval (grill CONFIRMED-5), not the
// pure function alone.
//
// **The loop is ended by the BUDGET here, never by a clock** (/pending 475). The test used to
// give the loop a 3 s context and a 1 ms interval and then want exactly the cap — which holds
// only where 1,500 one-millisecond timers per target fire inside 3 s. Beside another suite they
// do not ("spent 2413, want the cap 3000"; 20 of 20 red beside 48 spinning shells), and the
// message then blamed a bound nothing had broken. How long 3,000 sends take is a fact about
// the machine; that the 3,001st is never sent, and that the loop then RETURNS by itself, is
// the property. So: no interval, no deadline the loop can lose to, and an overshoot is caught
// at the send that makes it. The watchdog only turns a loop that never returns into a failure
// instead of a hung package, and it says which of the two it saw.
func TestPunchLoopEmitsBoundedByTheBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	hits := map[string]int{}
	total, over := 0, 0
	punch := func(a net.Addr) error {
		mu.Lock()
		defer mu.Unlock()
		hits[a.String()]++
		total++
		if total > punchBudgetPerSide {
			over++
			cancel() // a loop that ignores the budget must not run for ever
		}
		return nil
	}
	budget := &punchBudget{}
	cands := make(chan candidate, 4)
	cands <- candidate{Addr: "203.0.113.9:5000"}   // IPv4 target (documentation range is fine — not screened here)
	cands <- candidate{Addr: "[2001:db8::1]:5000"} // IPv6 — must NOT be punched
	cands <- candidate{Addr: "198.51.100.7:6000"}  // second IPv4 target
	close(cands)

	// One runnable goroutine gives the other target no turn until this one waits — and with no
	// interval it would otherwise never wait, so one target could spend the whole budget alone.
	yield := func(time.Duration) time.Duration { return 50 * time.Microsecond }
	done := make(chan struct{})
	go func() {
		defer close(done)
		punchLoop(ctx, punch, budget, cands, yield)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Minute):
		cancel()
		<-done
		spent, _ := budget.report()
		t.Fatalf("punchLoop had not returned after 5 minutes with %d of %d spent — it does not end when the budget does (or this machine cannot send 3,000 datagrams to a function in five minutes)", spent, punchBudgetPerSide)
	}

	mu.Lock()
	defer mu.Unlock()
	if over != 0 {
		t.Fatalf("punch was called %d time(s) past the cap of %d — the loop did not stop at the budget", over, punchBudgetPerSide)
	}
	if ctx.Err() != nil {
		t.Fatalf("the context was ended while the loop ran: %v", ctx.Err())
	}
	// The IPv6 candidate got nothing.
	if hits["[2001:db8::1]:5000"] != 0 {
		t.Errorf("an IPv6 candidate was punched %d times — v6 is dialled directly, no hole needed", hits["[2001:db8::1]:5000"])
	}
	// The two IPv4 targets shared the budget, and the total equals the cap: the loop returned
	// by itself, with the context still live, so nothing but the budget can have ended it.
	spent, dropped := budget.report()
	if total != spent {
		t.Errorf("punch emitted %d datagrams but the budget recorded %d spent", total, spent)
	}
	if spent != punchBudgetPerSide {
		t.Errorf("the loop returned by itself with %d spent, want the cap %d — something other than the budget ended it", spent, punchBudgetPerSide)
	}
	// Each target is refused exactly once and then stops asking: drop-and-report, not drop-and-retry.
	if dropped != 2 {
		t.Errorf("the budget recorded %d refused sends, want 2 — one per IPv4 target, each stopping at its first refusal", dropped)
	}
	if hits["203.0.113.9:5000"] == 0 || hits["198.51.100.7:6000"] == 0 {
		t.Errorf("one of the two IPv4 targets was never punched: %v", hits)
	}
}

// The loop stops promptly on context cancel, well before the budget — a ceremony that connects
// via a faster tier must not keep punching.
//
// **Cancelled at the tenth SEND, not after a sleep** (/pending 475): "well before the budget"
// was a 50 ms sleep against a 1 ms interval, i.e. a guess about how many sends 50 ms holds.
// Counted, the claim is exact: after the cancel the loop may finish the send it is in and no
// more, and it returns.
func TestPunchLoopStopsOnCancel(t *testing.T) {
	const cancelAt = 10
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var n int32
	punch := func(net.Addr) error {
		if atomic.AddInt32(&n, 1) == cancelAt {
			cancel()
		}
		return nil
	}
	budget := &punchBudget{}
	cands := make(chan candidate, 1)
	cands <- candidate{Addr: "203.0.113.9:5000"}
	// left open, like a live feed

	done := make(chan struct{})
	go func() {
		// An interval no test could wait out: after the cancel, only the context can end the wait.
		punchLoop(ctx, punch, budget, cands, func(time.Duration) time.Duration {
			if atomic.LoadInt32(&n) >= cancelAt {
				return time.Hour
			}
			return 0
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Minute):
		t.Fatal("punchLoop had not returned 5 minutes after its context was cancelled — it keeps punching after a faster tier won")
	}
	if got := atomic.LoadInt32(&n); got != cancelAt {
		t.Errorf("the loop sent %d datagrams, want exactly %d — the context was cancelled inside send %d and nothing may follow it", got, cancelAt, cancelAt)
	}
	if spent, _ := budget.report(); spent != cancelAt {
		t.Errorf("the budget recorded %d spent after a cancel at send %d", spent, cancelAt)
	}
}
