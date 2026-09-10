package server

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestAnArmLandingBeforeTheStreamSubscribesIsNotLost — /pending 464.
//
// # The defect, and it is production rather than a flake
//
// `handleWindow` used to read the armed state, marshal it, write it to the socket, flush — and only
// THEN call `armedChanges()` to wait. `armedChangedLocked` broadcasts by CLOSING that channel and
// setting it to nil, and `armedChanges` lazily makes a fresh one. So an arm landing anywhere in that
// window closed a channel the loop did not hold, and the subscribe afterwards handed back a new
// channel that would only fire on the NEXT change. The stream parked forever on a change that had
// already happened.
//
// A window open when a ceremony arms therefore never learned the machine was armed, so
// `beforeunload` reported nothing-to-lose while a ceremony was running — which is the policy-armed
// case D5 exists for, and what this stream was added to carry.
//
// It surfaced as `TestTheWindowStreamCarriesTheArmedState` failing under `go test ./...` twice on
// 2026-09-10 at its full 30s deadline, and passing alone in 0.089s: the window between the read and
// the subscribe is a network write, and it widens under load.
//
// # What this test drives, and what it cannot
//
// It drives the ORDER at the session, which is where the lost wakeup lives: take the channel, then
// change the state, and require the channel to have fired. Under the old handler ordering the
// channel was taken after the change and so was a fresh one — this is that, made deterministic
// rather than waiting for a loaded machine.
//
// **It cannot drive the handler's own interleaving**, because there is no seam to pause it between
// the read and the subscribe. That half is asserted over the SOURCE below, which is the same shape
// /pending 387 settled on when a black-box test came back 0 of 10 red across three shapes: an
// ordering that cannot be driven is asserted where it is written.
func TestAnArmLandingBeforeTheStreamSubscribesIsNotLost(t *testing.T) {
	se := &session{}

	// The channel taken FIRST, which is what the fixed handler does.
	changed := se.armedChanges()
	if !se.armIn(&arm{kind: armInteractive, addr: "127.0.0.1:0"}) {
		t.Fatal("setup: could not arm, so nothing below is a change")
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("a channel taken BEFORE the arm never fired. The broadcast closes the channel it " +
			"holds, so a waiter holding it must wake — if this fails the notify itself is broken " +
			"and the handler's ordering is not the question")
	}

	// And the shape that WAS the defect: a channel taken after the change is a fresh one, so it
	// carries no news of the change that already happened. Asserted so the reason the order
	// matters is visible here rather than only in a comment.
	stale := se.armedChanges()
	select {
	case <-stale:
		t.Error("a channel obtained AFTER the change fired immediately. That would mean the " +
			"broadcast is level-triggered, and the ordering this item is about would not matter " +
			"— read the notify again, because one of these two tests is now wrong")
	case <-time.After(100 * time.Millisecond):
		// Correct: edge-triggered, which is exactly why the subscribe must come first.
	}

	// The handler's half, over the source. `armedChanges()` must be called before `ArmedWhat()`
	// inside the stream loop, and the select must wait on the value captured there rather than
	// calling `armedChanges()` again.
	src, err := os.ReadFile("window.go")
	if err != nil {
		t.Fatal(err)
	}
	body := funcBodyFrom(string(src), strings.Index(string(src), "func (s *Server) handleWindow("))
	if !strings.Contains(body, "event: armed") {
		t.Fatal("handleWindow's body could not be read — the clauses below would pass over nothing")
	}
	sub, read := strings.Index(body, "s.sess.armedChanges()"), strings.Index(body, "s.sess.ArmedWhat()")
	if sub < 0 || read < 0 {
		t.Fatalf("the stream loop no longer both subscribes and reads (subscribe at %d, read at %d)", sub, read)
	}
	if sub > read {
		t.Error("handleWindow subscribes to armed changes AFTER reading the state. The broadcast " +
			"is edge-triggered — close and nil — so an arm landing between the read and the " +
			"subscribe is lost, and the stream waits for a change that has already happened. That " +
			"is /pending 464: a window open when a ceremony arms never learns it is armed, and " +
			"the close prompt says nothing-to-lose while a ceremony runs")
	}
	if strings.Count(body, "s.sess.armedChanges()") != 1 {
		t.Error("the stream loop calls armedChanges() more than once. Calling it again in the " +
			"select discards the channel taken at the top of the loop and reopens the same window")
	}
}
