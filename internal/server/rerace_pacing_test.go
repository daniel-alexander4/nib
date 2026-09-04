package server

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// /pending 369 — the pre-signing re-race had no backoff and nothing had ever measured its rate.
//
// **A bound answers the rate question that a measurement was going to answer.** The item asked for
// a tier-4 flapping-peer clause first, on the reasoning that there is nothing to design until the
// rate is known. But the loop's continue condition is a REMOTE peer failing, so the rate was never
// this side's to know — and a ceiling makes it irrelevant: once warmed the arm cannot re-enter more
// often than `reraceCap`, whatever the peer does.
func TestTheReraceIsPacedAndBounded(t *testing.T) {
	now := time.Now()
	far := now.Add(time.Hour)

	// It BACKS OFF: each attempt waits at least as long as the one before, and the first is
	// small enough to cost an honest re-race nothing.
	first, ok := reraceWait(0, now, far)
	if !ok {
		t.Fatal("the first re-race is refused inside a live window")
	}
	if first != reraceBase {
		t.Errorf("the first wait is %v, want %v — a legitimate re-race follows a peer that dropped "+
			"mid-handshake and needs longer than this to return, so the first wait must not be "+
			"the one that paces anybody", first, reraceBase)
	}
	prev := first
	for n := 1; n < 12; n++ {
		d, ok := reraceWait(n, now, far)
		if !ok {
			t.Fatalf("attempt %d refused inside a live window", n)
		}
		if d < prev {
			t.Errorf("attempt %d waits %v, less than attempt %d's %v — a backoff that goes "+
				"backwards paces nothing", n, d, n-1, prev)
		}
		prev = d
	}

	// And it is CAPPED, which is the half that bounds a hostile peer rather than a clumsy one.
	if prev != reraceCap {
		t.Errorf("after eleven attempts the wait is %v, want the ceiling %v. Without a ceiling "+
			"the delay grows until it swallows the window, and an arm that waits an hour between "+
			"attempts has stopped listening rather than started pacing", prev, reraceCap)
	}
	if huge, _ := reraceWait(1000, now, far); huge != reraceCap {
		t.Errorf("attempt 1000 waits %v, want %v — the shift must saturate, not overflow", huge, reraceCap)
	}
}

// The deadline half, which used to be a separate inline test at one site and absent at the other.
func TestTheReraceStopsAtItsDeadlineAndNeverSleepsPastIt(t *testing.T) {
	now := time.Now()

	if _, ok := reraceWait(0, now, now); ok {
		t.Error("a re-race is allowed at the instant the window closes. The deadline check and " +
			"the delay are one question through one door precisely so a site cannot get the " +
			"pacing and miss the bound")
	}
	if _, ok := reraceWait(0, now, now.Add(-time.Second)); ok {
		t.Error("a re-race is allowed after the window has closed")
	}

	// **The wait never outlives the window it is pacing.** A backoff that overshot would turn a
	// bounded arm into one that sleeps past its own end and wakes to discover it — and at
	// `reraceCap` against a window with 10ms left, the naive answer is 200x too long.
	d, ok := reraceWait(9, now, now.Add(10*time.Millisecond))
	if !ok {
		t.Fatal("a window with 10ms left refuses a re-race")
	}
	if d > 10*time.Millisecond {
		t.Errorf("the wait is %v against a window with 10ms left — it would wake after the arm "+
			"it is pacing had already ended", d)
	}
}

// sleepOrDone is the other half of honouring a teardown, and it is why the pacing is not a
// `time.Sleep`.
func TestPacingIsAbandonedTheMomentTheArmIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if sleepOrDone(ctx, reraceCap) {
		t.Error("a cancelled arm completed its wait. A bare sleep in a retry loop is a cancel the " +
			"arm does not honour: at the ceiling that is two seconds per attempt of a goroutine " +
			"outliving the session that owned it")
	}
	if el := time.Since(start); el > reraceCap/2 {
		t.Errorf("a cancelled arm took %v to give up, against a %v wait", el, reraceCap)
	}
	// And it still waits when nothing is cancelled, or it would pace nothing at all.
	if !sleepOrDone(context.Background(), time.Millisecond) {
		t.Error("a live arm reports its wait was abandoned")
	}
}

// TestBothRetryArmsRouteThroughTheReraceDoor is ADR-009's guard, in this package's own idiom:
// find the function, take its body, assert it routes.
//
// **It asserts ROUTING, not the delay each site chose, and that is the point.** There are two
// retry arms in `runCeremonyReceive` — the pre-signing re-race and the post-signing promote — and
// `/pending 369` named only the first. A guard that checked the pre-signing site's numbers would
// have said nothing about the second, which had no deadline test of its own at all.
//
// **A structural guard because nothing can drive this function**, and that is stated rather than
// implied: the loop is reached only from the QUIC ceremony arm and its continue needs a counterpart
// that completes the p2p handshake and then drops it, repeatedly. The repo already guards this same
// function this same way for `armWindowFor` (rearm_test.go), for the same reason.
func TestBothRetryArmsRouteThroughTheReraceDoor(t *testing.T) {
	b, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(b)
	const fn = "func (s *Server) runCeremonyReceive("
	i := strings.Index(code, fn)
	if i < 0 {
		t.Fatalf("cannot find %s — this guard is pinned to a function that no longer exists under "+
			"that name, so its clean result says nothing", fn)
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("the brace matcher read an empty body")
	}
	if n := strings.Count(body, "reraceWait("); n != 2 {
		t.Errorf("runCeremonyReceive calls reraceWait %d time(s), want 2 — one per retry arm. "+
			"An arm that retries without the door has no delay and no bound, which is exactly "+
			"the state /pending 369 found the pre-signing one in and the state the post-signing "+
			"one was in unnoticed", n)
	}
	if n := strings.Count(body, "sleepOrDone("); n != 2 {
		t.Errorf("runCeremonyReceive calls sleepOrDone %d time(s), want 2. A site that took a "+
			"delay from the door and did not wait it would be paced on paper only", n)
	}
	// The shape the door replaced must not come back beside it: a bare `continue` reached from a
	// transport-loss test with no wait in between.
	if strings.Contains(body, "if isTransportLoss(cerr) && time.Now().Before(") {
		t.Error("the pre-signing arm tests the deadline inline again. The deadline check and the " +
			"delay are one question through one door precisely so a site cannot take the second " +
			"and skip the first")
	}
}
