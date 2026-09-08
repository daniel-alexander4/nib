package server

import (
	"log"
	"net/http"
	"sync/atomic"
)

// A window's life is a CONNECTION, not a timer (PLAN-window-lifetime.md, D1).
//
// Nib has to know whether any window still exists, because a process whose last
// window is gone should not keep serving — closing the window used to leave nib
// running, still answering, still holding its port and its instance record.
//
// The signal is a socket the page holds open for exactly as long as it exists.
// **A heartbeat cannot do this job and was overturned during the slice's grill**:
// a background window has its timers throttled to roughly once a minute and can
// be frozen outright, so a timestamp cannot tell a MINIMISED window from a CLOSED
// one — and that is the only distinction the feature turns on. Getting it wrong
// exits nib under a working user, which is worse than the orphan it fixes. A
// closed page drops its connection at the OS level; a frozen one does not, and
// neither depends on the page being able to run JavaScript on time.
//
// This slice counts windows and nothing else. Nothing exits yet — that is
// P01.S04, and it reads the count this file maintains.

// liveWindows is the number of windows currently holding a stream open.
//
// Atomic rather than under `Server.mu`, which guards the vault and the CSRF
// token: a window connecting has nothing to do with the vault, and taking that
// mutex for the whole life of a stream would hold it for as long as the window
// exists.
//
// A COUNT, never a boolean (D7). "A window is connected" cannot distinguish one
// of two closing from the last one closing, and the exit decision P01.S04 builds
// on this turns on exactly that difference.
type liveWindows struct{ n atomic.Int64 }

// Live reports how many windows are currently holding a stream.
func (w *liveWindows) Live() int { return int(w.n.Load()) }

// These are the strings a tier-3 test greps out of nib's log, and they are the
// seam inventory's *Emitted string* for rows P1 and S1 — so they are literals
// here rather than assembled from a format verb per event.
//
// **Logged rather than published in `/api/status`, deliberately.** A tier-3 test
// drives the real binary in a real browser and cannot call a Go accessor, so the
// count needs an observable. Adding a field to the status response would put
// internal state into the client's public shape for a test's benefit; a log line
// is the honest instrument, and it is also the diagnosability a user can send
// back when nib will not quit on a machine nobody here can reach.
const (
	windowConnectedMsg = "window connected"
	windowGoneMsg      = "window gone"
	// idleExitArmedMsg is P01.S03's observable, in the same shape and for the same
	// reason: a harness cannot call a Go accessor, and this is the line that says
	// whether THIS process is one that would ever exit on an empty window count.
	//
	// **A prefix rather than a whole sentence**, because the value is what a test
	// greps and the two must not be assembled independently at each end.
	idleExitArmedMsg = "idle-exit armed="
)

// IdleExitDecision is D2's rule as a value, so it can be probed (P01.S03).
//
// **Two facts, not one, and reading only the first is a real defect rather than a simplification.**
// `noBrowserRequested` is the environment saying "do not open a window"; `openErr` is whether one
// actually opened. They differ on a real machine: `browser.Open` falls back from an app-mode window
// to a browser tab and errors only when NOTHING could launch — a locked profile, snap or flatpak
// confinement, an Edge policy. A process whose browser failed to start has no window, and arming it
// means exiting on a user whose report already begins "I double-clicked Nib and nothing happened".
//
// It is a function rather than an expression at the call site because the difference is invisible
// in the common case: with a working browser both readings agree, and every harness sets the
// variable, so nothing this repo runs would notice the wrong one. A door can be given a test; an
// `&&` inside `main` cannot.
func IdleExitDecision(noBrowserRequested bool, openErr error) bool {
	return !noBrowserRequested && openErr == nil
}

// ArmIdleExit records whether this process is one that waits for a window (D2, P01.S03).
//
// # The rule is exact and needs no allow-list
//
// **Armed only if THIS process launched a browser.** A process that never opened a window is never
// waiting for one, so a headless run — every harness, and the `NIB_ADDR` SSH-tunnel mode — is never
// a candidate for idle-exit. D2 settled this against a heuristic precisely so the set cannot drift:
// there is no list of "test-like" conditions to keep current, only the fact of having launched.
//
// **"Launched" means `browser.Open` returned no error, not that `NIB_NO_BROWSER` was unset.** Those
// differ on a real machine: `Open` falls back from an app-mode window to a browser tab and returns
// an error only when NOTHING could launch — a locked profile, snap confinement, an Edge policy. A
// process whose browser failed to start has no window and must not arm, and reading only the
// environment variable would arm it. The user's report in that case is "I double-clicked Nib and
// nothing happened"; exiting on them would be the second half of that sentence.
//
// # Nothing exits yet
//
// This slice arms a FLAG and logs it. The grace timer, its two cancels and the exit itself are
// P01.S04, which reads this and the count above. Kept separate on purpose: a slice that armed the
// exit before the cancels existed would exit Nib on a reload.
func (s *Server) ArmIdleExit(launchedBrowser bool) {
	s.idleExit.Store(launchedBrowser)
	log.Printf("%s%v", idleExitArmedMsg, launchedBrowser)
}

// IdleExitArmed reports whether this process would consider exiting when its last window closes.
// P01.S04 is its first behavioural reader; today it exists so the flag has one door rather than a
// bool passed down two call paths.
func (s *Server) IdleExitArmed() bool { return s.idleExit.Load() }

// handleWindow holds a stream open for the life of one window and counts it.
//
// It is guarded by requirePublicLoopback, NOT requireUnlocked (D3). A window
// sitting on the unlock screen is a real window; behind the unlocked guard it
// would hold no stream, and once P01.S04 lands nib would exit while the user was
// typing a passphrase.
//
// The response is an SSE stream because the browser gives it auto-reconnect for
// free, which matters once a grace period exists: a transient drop reconnects
// inside it instead of being read as a closed window. Nothing is ever sent on
// it. The bytes do not carry the signal — the socket does.
func (s *Server) handleWindow(w http.ResponseWriter, r *http.Request) {
	// Checked before anything is counted, because a ResponseWriter that cannot
	// flush cannot deliver the headers that make the client consider the stream
	// open — the page would sit waiting and the count would be a lie. Neither
	// securityHeaders nor loopbackOnly wraps the ResponseWriter, so this holds
	// today; it is asserted rather than assumed because a future middleware that
	// wrapped it would break the whole mechanism silently.
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	// Flushed immediately: until the headers reach it the client has not opened
	// anything, so a count incremented before this would lead the exit decision
	// by however long the write took.
	flusher.Flush()

	n := s.windows.n.Add(1)
	log.Printf("%s (%d open)", windowConnectedMsg, n)
	defer func() {
		left := s.windows.n.Add(-1)
		log.Printf("%s (%d open)", windowGoneMsg, left)
	}()

	// The whole handler. net/http cancels the request context when the peer
	// disconnects, so this returns the moment the window is gone — and not
	// before, however long that is. There is no read deadline to survive: the
	// server is constructed with no timeouts (cmd/nib/main.go).
	<-r.Context().Done()
}
