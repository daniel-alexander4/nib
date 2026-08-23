package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// TestTheRendezvousCadenceStepsDownButNeverStops — /pending 256.
//
// P05.S09b gave the receive arm a window of `MaxCeremonyLife` — thirty days — while
// `feedCandidates` polled at a flat five seconds, and a poll here is a full iterative DHT
// traversal fanned out to the routing table, not one datagram. `lan.go` computed the same harm
// for the LAN announcer and capped it; this half never got the same treatment.
func TestTheRendezvousCadenceStepsDownButNeverStops(t *testing.T) {
	// The dialing side is bounded by connectDeadline and must be UNCHANGED — its dozen polls in
	// a 300 s race are what the constant was sized for.
	for _, e := range []time.Duration{0, time.Second, connectDeadline - time.Second} {
		if got := rendezvousInterval(e); got != candidateFetchEvery {
			t.Errorf("rendezvousInterval(%v) = %v, want the unchanged %v — the step-down must not "+
				"touch the race the constant was sized for", e, got, candidateFetchEvery)
		}
	}
	// Past the race it must actually step down, or the arm keeps paying race rates for a month.
	if got := rendezvousInterval(connectDeadline + time.Second); got <= candidateFetchEvery {
		t.Errorf("rendezvousInterval just past the race = %v, want more than %v — nothing steps down",
			got, candidateFetchEvery)
	}
	// Monotone, and bounded above by the republish period so a side can never miss a whole
	// generation of its peer's record.
	prev := time.Duration(0)
	for _, e := range []time.Duration{0, connectDeadline, 10 * time.Minute, time.Hour, 30 * 24 * time.Hour} {
		got := rendezvousInterval(e)
		if got < prev {
			t.Errorf("rendezvousInterval(%v) = %v, less than the previous tier's %v — not monotone", e, got, prev)
		}
		if got > republishEvery() {
			t.Errorf("rendezvousInterval(%v) = %v, longer than the republish period %v — a side "+
				"fetching slower than its peer republishes can miss a whole generation of the record",
				e, got, republishEvery())
		}
		prev = got
	}
	// **And it must never become a CAP.** lan.go's announce cap is only safe because it delegates
	// late discovery to this loop; a zero here would leave a thirty-day arm discovering nothing.
	if rendezvousInterval(30*24*time.Hour) <= 0 {
		t.Error("the cadence reached zero or negative — the loop would spin or stop, and lan.go's " +
			"announce cap depends on this loop still running")
	}
}

// TestTheRepublishPeriodKeepsTheRecordAlive — the arithmetic half of /pending 256, and it is
// arithmetic on purpose: `MaxCandidateLife` is a READER-side ceiling every peer enforces, so no
// value of the record's own expiry can cover an arm. Republishing is the only mechanism there is.
func TestTheRepublishPeriodKeepsTheRecordAlive(t *testing.T) {
	if republishEvery() >= candidateLife() {
		t.Errorf("republish every %v against a record that lives %v — the record expires before it "+
			"is replaced, and the arm goes un-findable in the gap", republishEvery(), candidateLife())
	}
	// The ceiling that makes the previous assertion unsatisfiable by simply extending the record.
	if candidateLife() >= ceremony.MaxCandidateLife {
		t.Fatalf("setup: candidateLife %v is at or past the reader-side ceiling %v, so peers would "+
			"refuse the record outright", candidateLife(), ceremony.MaxCandidateLife)
	}
	// The gap this closes, stated as the number it is: an arm outliving one record generation.
	if ceremony.MaxCeremonyLife <= candidateLife() {
		t.Fatal("setup: the arm no longer outlives one record, so this item's premise is gone")
	}
}

// TestAnArmRepublishesForAsLongAsItIsArmed — the behavioural half.
//
// There is no fake clock in this package (punch.go says so at the line), so the loop takes its
// periods as parameters and the test drives them small — the `startAnnouncing` shape.
func TestAnArmRepublishesForAsLongAsItIsArmed(t *testing.T) {
	var n int32
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	publishLoop(ctx, 5*time.Millisecond, 20*time.Millisecond, func(context.Context) {
		atomic.AddInt32(&n, 1)
	})

	got := atomic.LoadInt32(&n)
	// SETUP: the first publish must have happened at all, or "more than one" is true for the
	// wrong reason and would stay true if the loop never started.
	if got < 1 {
		t.Fatal("setup: the loop published nothing, so the count below measures nothing")
	}
	if got < 3 {
		t.Errorf("the arm published %d times over ~12 periods — it publishes ONCE and the record "+
			"then expires, leaving the arm un-findable for the rest of its window", got)
	}
}

// TestTheFirstPublishStillWaitsOutTheLANWindow — the D6 suppression the loop must not break.
//
// A ceremony that completes over the LAN inside `browseWindow` must never write to the DHT: the
// write IS the correlation handle the arm exists to suppress. A republish loop that started
// before that window would re-open what criterion 10's amendment closed.
func TestTheFirstPublishStillWaitsOutTheLANWindow(t *testing.T) {
	var n int32
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel() // the faster tier won, inside the window
	}()
	publishLoop(ctx, time.Second, time.Millisecond, func(context.Context) {
		atomic.AddInt32(&n, 1)
	})
	if got := atomic.LoadInt32(&n); got != 0 {
		t.Errorf("the loop published %d time(s) despite the LAN window being won inside it — the "+
			"publish-write is the correlation handle the suppression exists to prevent", got)
	}
}
