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
func TestPunchLoopEmitsBoundedByTheBudget(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	punch := func(a net.Addr) error {
		mu.Lock()
		hits[a.String()]++
		mu.Unlock()
		return nil
	}
	budget := &punchBudget{}
	cands := make(chan candidate, 4)
	cands <- candidate{Addr: "203.0.113.9:5000"}   // IPv4 target (documentation range is fine — not screened here)
	cands <- candidate{Addr: "[2001:db8::1]:5000"} // IPv6 — must NOT be punched
	cands <- candidate{Addr: "198.51.100.7:6000"}  // second IPv4 target
	close(cands)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	fast := func(time.Duration) time.Duration { return time.Millisecond }
	punchLoop(ctx, punch, budget, cands, fast)

	mu.Lock()
	defer mu.Unlock()
	// The IPv6 candidate got nothing.
	if hits["[2001:db8::1]:5000"] != 0 {
		t.Errorf("an IPv6 candidate was punched %d times — v6 is dialled directly, no hole needed", hits["[2001:db8::1]:5000"])
	}
	// The two IPv4 targets shared the budget, and the total equals the cap (the loop ran until
	// the budget was spent, since the fast interval keeps it going past 3000 inside 3s).
	total := 0
	for _, n := range hits {
		total += n
	}
	spent, _ := budget.report()
	if total != spent {
		t.Errorf("punch emitted %d datagrams but the budget recorded %d spent", total, spent)
	}
	if spent != punchBudgetPerSide {
		t.Errorf("the shared budget spent %d, want the cap %d — the loop did not stop at the budget", spent, punchBudgetPerSide)
	}
	if hits["203.0.113.9:5000"] == 0 || hits["198.51.100.7:6000"] == 0 {
		t.Errorf("one of the two IPv4 targets was never punched: %v", hits)
	}
}

// The loop stops promptly on context cancel, well before the budget — a ceremony that connects
// via a faster tier must not keep punching.
func TestPunchLoopStopsOnCancel(t *testing.T) {
	var n int32
	punch := func(net.Addr) error { atomic.AddInt32(&n, 1); return nil }
	budget := &punchBudget{}
	cands := make(chan candidate, 1)
	cands <- candidate{Addr: "203.0.113.9:5000"}
	// left open, like a live feed

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		punchLoop(ctx, punch, budget, cands, func(time.Duration) time.Duration { return time.Millisecond })
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("punchLoop did not stop within 2s of cancel — it keeps punching after a faster tier won")
	}
	if spent, _ := budget.report(); spent >= punchBudgetPerSide {
		t.Errorf("the loop spent the whole budget (%d) despite an early cancel", spent)
	}
}
