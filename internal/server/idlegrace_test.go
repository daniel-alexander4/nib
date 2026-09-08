package server

import (
	"testing"
	"time"
)

// gracedServer is a server whose grace is short enough to drive, armed as a launched process.
func gracedServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{}
	s.ArmIdleExit(true)
	return s
}

// TestTheGraceArmsOnTheTRANSITIONAndNotOnAZeroCount — P01.S04 T01, and the trap the slice's own
// text did not name.
//
// At startup the window count is zero before the first window connects. A rule reading "the count
// is zero" therefore fires during BOOT — before the browser this process just launched has
// finished loading — and exits Nib under a user who has seen nothing. `handleWindow`'s
// `left := Add(-1)` is the transition itself, which makes the initial zero unreachable rather than
// merely unlikely.
func TestTheGraceArmsOnTheTRANSITIONAndNotOnAZeroCount(t *testing.T) {
	s := gracedServer(t)
	// A brand-new server has a zero count and must NOT be counting down.
	select {
	case <-s.IdleExit():
		t.Fatal("a server that has never had a window is already exiting. The count is zero at " +
			"boot, before the browser this process launched has connected — arming there quits " +
			"Nib under a user who has seen nothing")
	default:
	}
	if s.idleGraceRunning() {
		t.Error("the grace is armed on a server with no window history. It must arm on the 1->0 " +
			"TRANSITION, which is unreachable before a window has ever connected")
	}
}

// TestTheGraceFiresWhenNoWindowComesBack — the exit cause itself.
func TestTheGraceFiresWhenNoWindowComesBack(t *testing.T) {
	s := gracedServer(t)
	// Drive the transition the handler drives, then shorten the wait by firing the timer's work
	// directly is NOT what this does — it arms for real and waits, because the thing under test is
	// that the timer fires at all.
	s.windows.n.Store(1)
	left := s.windows.n.Add(-1)
	if left != 0 {
		t.Fatalf("setup: the count is %d after the last window left", left)
	}
	s.armIdleExitGrace()
	if !s.idleGraceRunning() {
		t.Fatal("the last window left and no grace started, so the process would keep running " +
			"with no window — which is the defect this whole plan exists for")
	}
	// The timer is real; assert it is COUNTING rather than waiting out the full grace here.
	select {
	case <-s.IdleExit():
		t.Error("the grace fired immediately. A window that drops and reconnects — every reload — " +
			"would exit Nib under the user")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestBothCancelsStopTheGraceAndAreCountedApart — T02, T03 and D4's own words: "they are counted
// separately because they fail differently."
//
// A window cancelling is the ordinary case: a reload, a second window. A hand-off cancelling is the
// race the grill surfaced — close, relaunch immediately, and the file is handed to a process
// already on its way out. Folding the counters makes the rare one invisible inside the common one.
func TestBothCancelsStopTheGraceAndAreCountedApart(t *testing.T) {
	s := gracedServer(t)

	// An ordinary connect with no grace running must NOT count a cancel.
	if s.cancelIdleExit(idleExitCauseWindow) {
		t.Error("a cancel was reported with no grace running. Every window connect would then " +
			"count a cancel that did not happen, and the counter would measure connects")
	}
	if w, h := s.IdleExitCancels(); w != 0 || h != 0 {
		t.Errorf("counters are %d/%d before anything was cancelled", w, h)
	}

	// Cancel one: a window.
	s.armIdleExitGrace()
	if !s.cancelIdleExit(idleExitCauseWindow) {
		t.Fatal("a window did not cancel a running grace, so a reload exits Nib")
	}
	if w, h := s.IdleExitCancels(); w != 1 || h != 0 {
		t.Errorf("after a window cancel the counters are %d/%d, want 1/0 — the causes are kept "+
			"apart because they fail differently (D4)", w, h)
	}
	select {
	case <-s.IdleExit():
		t.Error("the grace fired after being cancelled by a window")
	case <-time.After(20 * time.Millisecond):
	}

	// Cancel two: a hand-off, counted on its own axis.
	s.armIdleExitGrace()
	if !s.cancelIdleExit(idleExitCauseHandoff) {
		t.Fatal("a hand-off did not cancel a running grace: the file would be handed to a process " +
			"that is already exiting, and the user would be left looking at a dead tab")
	}
	if w, h := s.IdleExitCancels(); w != 1 || h != 1 {
		t.Errorf("after a hand-off cancel the counters are %d/%d, want 1/1 — a hand-off counted "+
			"as a window cancel is the rare cause hidden inside the common one", w, h)
	}
}

// TestAnUnarmedProcessNeverStartsAGrace — D2 reaching through to D4.
//
// Every harness, and the NIB_ADDR tunnel mode, launched no window. A grace there would exit the
// process for want of a window that was never coming — mid-test, surfacing as a connection refused
// somewhere unrelated.
func TestAnUnarmedProcessNeverStartsAGrace(t *testing.T) {
	s := &Server{} // never armed: no browser was launched
	s.windows.n.Store(1)
	if left := s.windows.n.Add(-1); left != 0 {
		t.Fatalf("setup: count is %d", left)
	}
	s.armIdleExitGrace()
	if s.idleGraceRunning() {
		t.Fatal("an UNARMED process started a grace when its window count reached zero. It never " +
			"launched a window, so it is not waiting for one (D2) — this is every harness")
	}
	select {
	case <-s.IdleExit():
		t.Error("an unarmed process is exiting for want of a window it never had")
	case <-time.After(20 * time.Millisecond):
	}
}

// TestTheGraceIsMeasuredAgainstAReconnectNibChooses — the first acceptance clause, as a property.
//
// The clause asks for the reconnect to be "measured against the grace rather than assumed". A bare
// EventSource reconnects on the BROWSER's default — 3 s in Chromium, unstated elsewhere and free to
// change — so a grace picked against it is a margin against somebody else's constant. The stream
// sends an explicit `retry:`, so both numbers are Nib's and the margin is a stated ratio.
func TestTheGraceIsMeasuredAgainstAReconnectNibChooses(t *testing.T) {
	if sseRetry <= 0 {
		t.Fatal("the stream declares no retry, so the reconnect gap is the browser's default and " +
			"the grace below is a margin against a constant this repo does not own")
	}
	if idleExitGrace <= sseRetry {
		t.Fatalf("the grace (%s) is not longer than the reconnect it must absorb (%s), so an "+
			"ordinary reload exits Nib", idleExitGrace, sseRetry)
	}
	// The ratio, not merely the ordering. The reconnect is the FLOOR of the gap and not the gap: a
	// reload re-parses the document, and a loaded machine spends the difference.
	if idleExitGrace < 5*sseRetry {
		t.Errorf("the grace is %s against a %s reconnect — under 5x. The reconnect is the floor of "+
			"the gap, not the gap; being generous costs a few seconds nobody can see, being tight "+
			"exits Nib under a user who reloaded", idleExitGrace, sseRetry)
	}
}

// TestTheGraceIsArmedAndCancelledThroughTheRealRoute — P01.S04, and this is the clause's real home.
//
// # Why here and not tier 3
//
// The first acceptance clause is "a reload does not exit Nib; the reconnect is measured against the
// grace rather than assumed". **Tier 3 structurally cannot show it**: every harness runs
// `NIB_NO_BROWSER=1`, so `IdleExitArmed()` is false there and no grace is ever armed — which is
// S03's guard working, not a gap, because an armed harness would exit mid-run. A tier-3 test
// asserting "the grace was cancelled" would assert a line that tier can never emit, and one
// asserting "nib survived the reload" would pass against a build with no grace at all.
//
// So the arithmetic is driven HERE, through the real handler and a real HTTP client — the stream
// drops, the grace arms, a new stream cancels it — and tier 3 asserts the one thing only a browser
// adds: that a reload really does drop and re-open the stream.
func TestTheGraceIsArmedAndCancelledThroughTheRealRoute(t *testing.T) {
	ts, s := startServerWith(t)
	s.ArmIdleExit(true) // a process that launched a window, which no harness is

	closeIt := openWindow(t, ts.URL)
	waitForWindows(t, s, 1)

	// STIMULUS: nothing is counting down while a window is open. Without this the cancel below
	// could be cancelling a grace that had been running since before the window existed.
	if s.idleGraceRunning() {
		t.Fatal("a grace is running while a window is open")
	}

	// The window goes away — the 1 -> 0 transition a reload produces.
	closeIt()
	waitForWindows(t, s, 0)
	deadline := time.Now().Add(2 * time.Second)
	for !s.idleGraceRunning() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !s.idleGraceRunning() {
		t.Fatal("the last window's stream dropped and no grace was armed, so this process would " +
			"keep serving with no window — the defect the whole plan exists for")
	}

	// The reconnect — what a reload does a moment later — cancels it.
	back := openWindow(t, ts.URL)
	defer back()
	waitForWindows(t, s, 1)
	if s.idleGraceRunning() {
		t.Error("a window came back and the grace is still counting down. Every reload would " +
			"exit Nib after the grace, under a user who did nothing but refresh")
	}
	byWindow, byHandoff := s.IdleExitCancels()
	if byWindow != 1 {
		t.Errorf("the reconnect counted %d window cancels, want 1 — the counter is what makes "+
			"'the reconnect is measured against the grace' an observation rather than a hope", byWindow)
	}
	if byHandoff != 0 {
		t.Errorf("a window reconnect was counted as a HAND-OFF cancel (%d). D4 keeps the causes "+
			"apart because they fail differently", byHandoff)
	}
	// And the process did not exit.
	select {
	case <-s.IdleExit():
		t.Error("the exit fired even though a window came back inside the grace")
	default:
	}
}

// TestAGraceIsNeverArmedWhileAWindowIsOpen — the interleaving the route-level test cannot drive.
//
// The arming caller has already decremented, but a NEW window can connect between that decrement
// and the lock: it cancels a grace that does not exist yet, and is then counted. Arming on the
// caller's stale view would leave a grace running with a window open, and the process would exit
// under it a grace later.
//
// **Driven directly rather than by racing**, because the interleaving is rare enough that a
// concurrent test would pass on a broken build most of the time — the shape that makes a flaky
// green look like a pass. The property is "the count is the authority at the moment the timer is
// set", and that is checkable without a race at all.
func TestAGraceIsNeverArmedWhileAWindowIsOpen(t *testing.T) {
	s := gracedServer(t)
	s.windows.n.Store(1) // a window connected between the decrement and this call
	s.armIdleExitGrace()
	if s.idleGraceRunning() {
		t.Error("a grace was armed while a window is open. The arming caller's view is stale by " +
			"the time it takes the lock — a window that connects in that gap cancels a grace that " +
			"does not exist yet and is then counted, and this process would exit under it one " +
			"grace later")
	}
	// And the ordinary path still arms, or the guard above is just "never arm".
	s.windows.n.Store(0)
	s.armIdleExitGrace()
	if !s.idleGraceRunning() {
		t.Error("no grace is armed with the count at zero, so the guard has become 'never arm' — " +
			"the process would never exit and the whole plan is inert")
	}
}
