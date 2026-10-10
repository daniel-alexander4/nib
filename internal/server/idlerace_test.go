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

	s.fireGrace() // the grace's end, now rather than ten seconds from now
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
	s.fireGrace()
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
	if n, _ := s.windowArrived(); n != 1 {
		t.Errorf("windowArrived returned %d, want 1", n)
	}
	if !ran {
		t.Fatal("stimulus: the hook never ran, so nothing above was observed")
	}
}

// fireGrace runs the body of the grace that is armed now, as its timer would.
func (s *Server) fireGrace() {
	s.idleGraceElapsed(s.armedGrace())
}

// armedGrace reads which grace is armed, under the lock that owns it.
func (s *Server) armedGrace() uint64 {
	s.idle.mu.Lock()
	defer s.idle.mu.Unlock()
	return s.idle.gen
}

// TestACancelledGraceDoesNotEndTheOneArmedAfterIt — /pending 784 part 2.
//
// The last window closes and a grace is armed. Its timer fires and its body waits for `idle.mu`
// while a window arrives — cancelling it; `Stop` is too late for a body already running — and
// leaves again, arming a second grace. The first body then takes the lock. It used to find a timer
// set and no window, and exit: the second close got no grace, so a reload landing in that moment
// came back to a process already going. Driven step by step, as the test above is.
func TestACancelledGraceDoesNotEndTheOneArmedAfterIt(t *testing.T) {
	s := gracedServer(t)
	s.armIdleExitGrace() // the last window went
	first := s.armedGrace()
	s.windowArrived() // a window comes back: the first grace is cancelled
	// STIMULUS: the cancel really took a running grace, or the body below is not a cancelled one.
	if byWindow, _ := s.IdleExitCancels(); byWindow != 1 {
		t.Fatalf("setup: %d graces were cancelled by a window, want 1", byWindow)
	}
	s.windows.n.Add(-1)
	s.armIdleExitGrace() // and goes again: a second grace
	if !s.idleGraceRunning() || s.armedGrace() == first {
		t.Fatal("setup: no second grace was armed, so this test is not in the interleaving")
	}

	s.idleGraceElapsed(first) // the cancelled body, getting the lock only now
	select {
	case <-s.IdleExit():
		t.Fatal("a grace a window had cancelled ended the grace armed after it: the process exits " +
			"the moment the second window closes, with no grace for it to come back in")
	default:
	}
	if !s.idleGraceRunning() {
		t.Error("the cancelled body cleared the second grace, so nothing will ever exit this process")
	}

	// And the second grace's own body still exits, or the check has become 'never exit'.
	s.fireGrace()
	select {
	case <-s.IdleExit():
	default:
		t.Error("the grace that IS armed ended with no window and did not exit")
	}
}
