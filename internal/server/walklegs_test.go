package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTheRoundRunsLegsConcurrentlyAndBounded — /pending 376.
//
// # What this is for
//
// One party that is not listening burned `connectDeadline` (300 s) before the next leg started,
// and at `ceremony.MaxRoster` of 32 that is a ceiling of over two hours on one POST — so one
// unreachable party delayed EVERY other party's copy. `walkLegs` is the whole of the fix, and it
// is the only part of the round drivable without four processes and a DHT.
//
// # Both halves, and neither alone is the property
//
// Concurrency without a bound is a different defect, and a bound that is never reached proves the
// legs are not concurrent at all. So this asserts the observed maximum in flight is GREATER than
// one and NO GREATER than the width — measured from the run function itself rather than inferred
// from timing, because a timing-based assertion passes on a machine that happens to be slow.
func TestTheRoundRunsLegsConcurrentlyAndBounded(t *testing.T) {
	const n, width = 12, 4

	var inFlight, maxSeen atomic.Int64
	var ran sync.Map
	release := make(chan struct{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		walkLegs(context.Background(), width, n, func(i int) {
			cur := inFlight.Add(1)
			for {
				old := maxSeen.Load()
				if cur <= old || maxSeen.CompareAndSwap(old, cur) {
					break
				}
			}
			ran.Store(i, true)
			<-release // hold every leg open so the bound is observable rather than inferred
			inFlight.Add(-1)
		})
	}()

	// Wait for the bound to be reached, so the assertion is about a full pipe and not a race.
	deadline := time.Now().Add(5 * time.Second)
	for maxSeen.Load() < int64(width) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := maxSeen.Load()
	close(release)
	<-done

	if got <= 1 {
		t.Errorf("the round never had more than %d leg(s) in flight — it is still serial, which "+
			"is the whole of /pending 376: one unreachable party burns connectDeadline before "+
			"the next leg starts, and at MaxRoster 32 that is over two hours on one POST", got)
	}
	if got > width {
		t.Errorf("%d legs were in flight at once against a width of %d. Unbounded concurrency is "+
			"a different defect from the serial round, not a fix for it — the round shares ONE "+
			"endpoint and one send limiter, and the bound is what keeps that shape", got, width)
	}

	// Every leg ran, exactly once. A bound that drops work is worse than a serial round.
	count := 0
	ran.Range(func(any, any) bool { count++; return true })
	if count != n {
		t.Errorf("%d of %d legs ran; a party whose leg was never attempted is reported with "+
			"neither a delivery nor a reason", count, n)
	}
}

// TestACancelledRoundStopsStartingLegs — the round stops when the caller goes away (/pending 355),
// and the concurrent walk must not lose that.
//
// It asserts the walk stops STARTING legs, not that it abandons running ones: a leg in flight owns
// a connection and a `beginLeg` row, and dropping it would leave the row set and the peer
// mid-exchange.
func TestACancelledRoundStopsStartingLegs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int64
	walkLegs(ctx, 2, 50, func(int) {
		if started.Add(1) == 1 {
			cancel() // the caller disconnects while the round is walking
		}
		time.Sleep(2 * time.Millisecond)
	})
	got := started.Load()
	if got == 0 {
		t.Fatal("setup: no leg ran at all, so the assertion below is not about cancellation")
	}
	if got == 50 {
		t.Errorf("all 50 legs ran after the round was cancelled. There is nothing to learn from " +
			"a leg whose result nobody will read, and each one still costs a round trip per " +
			"remaining party (/pending 355)")
	}
}

// TestWalkLegsRunsEveryLegWhenWidthIsOne — the serial case is not a special case.
//
// A width of 1 must behave exactly as the round did before, because that is what makes
// `deliveryRoundWidth` a tuning figure rather than a switch between two code paths.
func TestWalkLegsRunsEveryLegWhenWidthIsOne(t *testing.T) {
	var order []int
	var mu sync.Mutex
	var maxSeen, inFlight atomic.Int64
	walkLegs(context.Background(), 1, 6, func(i int) {
		cur := inFlight.Add(1)
		if cur > maxSeen.Load() {
			maxSeen.Store(cur)
		}
		mu.Lock()
		order = append(order, i)
		mu.Unlock()
		time.Sleep(time.Millisecond)
		inFlight.Add(-1)
	})
	if maxSeen.Load() != 1 {
		t.Errorf("width 1 ran %d legs at once; it must be exactly the serial round", maxSeen.Load())
	}
	for i, v := range order {
		if v != i {
			t.Errorf("width 1 ran leg %d at position %d — the serial case must keep roster order, "+
				"which is what the outcome list the convener reads is drawn from", v, i)
		}
	}
	// A width of zero or less is a misconfiguration, not a request for no work.
	var ran int
	walkLegs(context.Background(), 0, 3, func(int) { ran++ })
	if ran != 3 {
		t.Errorf("width 0 ran %d of 3 legs — a bad width must clamp to serial, never to a round "+
			"that silently delivers to nobody", ran)
	}
}
