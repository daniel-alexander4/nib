package server

import (
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
