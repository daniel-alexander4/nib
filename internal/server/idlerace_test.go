package server

import "testing"

// TestAGraceThatEndsWithAWindowOpenDoesNotExit — /pending 499, the interleaving the review
// reproduced at v1.129.118, driven step by step rather than raced.
//
// Window B cancels (there is no grace yet), window A — the last one leaving — arms on a count of
// zero, and only then does B's count move. The grace is now running with a window open. Nothing
// at arm time can see this: the arm read the count correctly, and the count changed afterwards.
// The timer body is therefore where the count must be read, and this drives that body directly.
func TestAGraceThatEndsWithAWindowOpenDoesNotExit(t *testing.T) {
	s := gracedServer(t)
	s.cancelIdleExit(idleExitCauseWindow) // B: cancel, no grace to stop
	s.armIdleExitGrace()                  // A: last window gone, count zero
	// STIMULUS: the grace really is running, or the exit check below passes for want of one.
	if !s.idleGraceRunning() {
		t.Fatal("setup: no grace was armed on a zero count, so this test is not in the interleaving")
	}
	s.windows.n.Add(1) // B: its count moves

	s.idleGraceElapsed() // the grace's end, now rather than ten seconds from now
	select {
	case <-s.IdleExit():
		t.Fatal("the grace ended with a window open and the process exited anyway. The timer " +
			"never re-reads the count, so a window that connects as the last one closes is shut " +
			"down one grace later")
	default:
	}
	if s.idleGraceRunning() {
		t.Error("a grace that stood down left its timer set, so the next 1->0 transition cannot arm")
	}

	// And with no window the same body exits, or the re-check has become 'never exit'.
	s.windows.n.Store(0)
	s.armIdleExitGrace()
	s.idleGraceElapsed()
	select {
	case <-s.IdleExit():
	default:
		t.Error("the grace ended with no window and did not exit — the stand-down swallowed the " +
			"ordinary case")
	}
}

// TestAWindowsCancelAndCountShareOneLockHold — the other half of /pending 499's fix.
//
// The timer's re-read is only authoritative if no window can be between its cancel and its count
// when the read happens. That is a property of lock holds, not of timing, so it is observed from
// inside the hold: while windowArrived sits between the two steps, the lock must not be free.
func TestAWindowsCancelAndCountShareOneLockHold(t *testing.T) {
	s := gracedServer(t)
	ran := false
	s.idle.arrivedHook = func() {
		ran = true
		if s.idle.mu.TryLock() {
			s.idle.mu.Unlock()
			t.Error("idle.mu was free between a window's cancel and its count, so a grace can " +
				"arm, or fire, reading a count that has not yet moved for a window already here")
		}
		if got := s.windows.Live(); got != 0 {
			t.Errorf("the count moved before the cancel (%d); the cancel must come first", got)
		}
	}
	if n := s.windowArrived(); n != 1 {
		t.Errorf("windowArrived returned %d, want 1", n)
	}
	if !ran {
		t.Fatal("stimulus: the hook never ran, so nothing above was observed")
	}
}
