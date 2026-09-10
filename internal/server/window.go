package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"nib/internal/safe"
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
func (s *Server) IdleExitArmed() bool { return s.idleExit.Load() }

// ── The grace, and its two cancels (P01.S04, D4) ────────────────────────────────

// idleExitGrace is how long the last window has to come back before the process gives up on it.
//
// **It is measured against a reconnect Nib CHOOSES, not one the browser picks** (T05). A bare
// `EventSource` reconnects on the browser's own default — 3 s in Chromium, unstated elsewhere and
// free to change — so a grace picked against it would be a guess against somebody else's constant.
// The stream sends an explicit `retry:` instead, so both numbers are ours and the margin is a
// stated ratio rather than a hope. `sseRetry` is that number; this is ten times it.
//
// **Ten times and not twice**, because the reconnect is the *floor* of the gap and not the gap: a
// reload re-parses the document, and a machine under load can spend the difference. The cost of
// being generous is that a closed window's process lingers for a few seconds with no window, which
// nobody can see; the cost of being tight is exiting under a user who reloaded.
const (
	idleExitGrace = 10 * time.Second
	sseRetry      = 1 * time.Second
)

// These are the strings a tier-3 test greps, on `windowConnectedMsg`'s footing.
//
// The cancel line carries the ELAPSED time (T06) because the first acceptance clause asks for the
// reconnect to be *measured* against the grace rather than asserted to be under it. A line saying
// only "cancelled" would pass whether the reconnect took 200 ms or 9.9 s.
const (
	idleExitGraceMsg     = "no windows left; exiting in"
	idleExitCancelMsg    = "idle-exit cancelled by"
	idleExitFiringMsg    = "no window came back; exiting"
	idleExitCauseWindow  = "a window"
	idleExitCauseHandoff = "a hand-off"
)

// armedEvent is what the window stream pushes: whether this machine is armed, and what for.
//
// **A JSON object rather than the bare bool P01.S05 shipped**, because S06's acceptance clause is
// that the modal names the ceremony specifically. `What` is empty for a manual co-signing arm,
// which has no ceremony to name, and the client says so in its own words rather than printing an
// empty string.
type armedEvent struct {
	Armed bool   `json:"armed"`
	What  string `json:"what,omitempty"`
}

// idleExitTimer is the grace and the two counters D4 asks to be kept apart.
//
// **Two counters, not one**, in D4's own words: "they are counted separately because they fail
// differently." A window cancelling is the ordinary case — a reload, a second window. A hand-off
// cancelling is the race the grill surfaced and neither party had named: close, relaunch
// immediately, and the file is handed to a process that is already on its way out. Folding them
// would make the rare one invisible inside the common one.
type idleExitTimer struct {
	mu        sync.Mutex
	timer     *time.Timer
	armedAt   time.Time
	fired     chan struct{}
	exited    bool // RequestExit is idempotent: two causes can race, and close() twice panics
	byWindow  atomic.Uint64
	byHandoff atomic.Uint64
}

// IdleExit is the channel `run()` selects on as its THIRD exit cause.
//
// **A channel and not a teardown call, and that is D6.** `run()`'s teardown is four steps and only
// two of them are visible there — `DisarmSession()` and `srv.Close()` run inline, then the LIFO
// defers `stop()` and `instance.Remove(cfgDir)` — which is why `main()` is `os.Exit(run())` at all.
// A third *cause* that called teardown itself would be a third teardown, and the failure that
// prevents is the stale instance record returning by a new door (ADR-009).
// Exit causes, for the log line that says WHY this process is going.
const (
	exitCauseLastWindow = "the last window closed"
	exitCauseQuit       = "you chose Quit"
)

// RequestExit is the ONE door onto "this process should exit" (P01.S06, D6).
//
// **A second closer of this channel would be a second exit path in everything but name.** D6's
// rule is that every exit path runs the same teardown in the same order, and the way that is kept
// true is that nothing tears down for itself: causes signal here, `run()` selects, and the teardown
// below it is the only one there has ever been. The grace (P01.S04) was closing the channel
// directly until Quit needed to as well — which is exactly when a rule like this earns its keep.
//
// Idempotent, because two causes can genuinely race: a user pressing Quit as the grace elapses is
// ordinary, not pathological, and `close` on a closed channel panics.
func (s *Server) RequestExit(cause string) {
	s.idle.mu.Lock()
	defer s.idle.mu.Unlock()
	if s.idle.exited {
		return
	}
	s.idle.exited = true
	if s.idle.fired == nil {
		s.idle.fired = make(chan struct{})
	}
	log.Printf("%s%s", exitingMsg, cause)
	close(s.idle.fired)
}

// exitingMsg is the observable for WHY the process is exiting — the seam a user's log carries when
// Nib went away and they want to know what decided that.
const exitingMsg = "exiting: "

func (s *Server) IdleExit() <-chan struct{} {
	s.idle.mu.Lock()
	defer s.idle.mu.Unlock()
	if s.idle.fired == nil {
		s.idle.fired = make(chan struct{})
	}
	return s.idle.fired
}

// armIdleExitGrace starts the grace. Called ONLY on the 1→0 window transition.
//
// **The transition and not the count, and the difference is a boot that exits.** At startup the
// count is zero before the first window connects, so a rule reading "the count is zero" would fire
// during boot, before the browser this process just launched had finished loading. `handleWindow`'s
// `left := Add(-1)` is the transition itself, and it makes the initial zero unreachable rather than
// merely unlikely.
func (s *Server) armIdleExitGrace() {
	if !s.IdleExitArmed() {
		return
	}
	s.idle.mu.Lock()
	defer s.idle.mu.Unlock()
	if s.idle.timer != nil {
		return // already counting down; a second 1->0 cannot happen, but nothing relies on that
	}
	// **Re-checked here, and this closes a real interleaving.** The arming caller has already
	// decremented, but a NEW window can connect between that decrement and this lock — cancelling
	// a grace that does not exist yet and then being counted. Arming on the caller's stale view
	// would leave a grace running with a window open, and the process would exit under it a grace
	// later. The count is the authority at the moment the timer is set, not at the moment the
	// decision to set it was taken.
	if s.windows.Live() != 0 {
		return
	}
	if s.idle.fired == nil {
		s.idle.fired = make(chan struct{})
	}
	s.idle.armedAt = time.Now()
	log.Printf("%s %s", idleExitGraceMsg, idleExitGrace)
	s.idle.timer = time.AfterFunc(idleExitGrace, func() {
		defer safe.Recover("idle exit")
		s.idle.mu.Lock()
		// Re-checked under the lock: `AfterFunc` can already be running when `cancelIdleExit`
		// takes the lock, and `Stop` returning false is exactly that case. Clearing the field is
		// what the cancel observes, so a fire that finds it nil has been cancelled and must not
		// close the channel.
		if s.idle.timer == nil {
			s.idle.mu.Unlock()
			return
		}
		s.idle.timer = nil
		s.idle.mu.Unlock()
		log.Printf("%s", idleExitFiringMsg)
		s.RequestExit(exitCauseLastWindow)
	})
}

// cancelIdleExit stops a running grace and counts WHY. Returns whether there was one to stop, so an
// ordinary window connect does not count a cancel that did not happen.
func (s *Server) cancelIdleExit(cause string) bool {
	// **The stop and its count happen under ONE lock hold, and they did not at first.** Clearing
	// the timer and then counting leaves a window in which the grace is cancelled and the counter
	// has not moved — so an observer that checks "is it cancelled" and then reads the cause sees a
	// cancel attributed to nobody. A test caught it; the counters are a diagnostic, so nothing
	// would have failed in production, and that is exactly the kind of gap that survives.
	s.idle.mu.Lock()
	t, at := s.idle.timer, s.idle.armedAt
	s.idle.timer = nil
	if t != nil {
		switch cause {
		case idleExitCauseHandoff:
			s.idle.byHandoff.Add(1)
		default:
			s.idle.byWindow.Add(1)
		}
	}
	s.idle.mu.Unlock()
	if t == nil {
		return false
	}
	t.Stop()
	log.Printf("%s %s after %s (grace %s)", idleExitCancelMsg, cause, time.Since(at).Round(time.Millisecond), idleExitGrace)
	return true
}

// idleGraceRunning reports whether a grace is counting down, under the lock that owns the timer.
//
// **It exists because the race detector caught its absence.** The tests read `s.idle.timer`
// directly while the stream handler wrote it from another goroutine — a real data race, in the
// tests rather than the product, and exactly what `-race` is for. A field guarded by a mutex has
// one reader shape and this is it; reaching past the lock because "it's only a test" is how the
// guarded field stops being guarded.
func (s *Server) idleGraceRunning() bool {
	s.idle.mu.Lock()
	defer s.idle.mu.Unlock()
	return s.idle.timer != nil
}

// IdleExitCancels reports the two counters, kept apart per D4.
func (s *Server) IdleExitCancels() (byWindow, byHandoff uint64) {
	return s.idle.byWindow.Load(), s.idle.byHandoff.Load()
}

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

	// **The reconnect gap is Nib's number** (T05). Without this the browser picks it — 3 s in
	// Chromium, unstated elsewhere — and `idleExitGrace` would be a margin against somebody else's
	// constant. Written before the count moves, so a client that reads it and immediately drops
	// still has it.
	_, _ = w.Write([]byte("retry: " + strconv.Itoa(int(sseRetry/time.Millisecond)) + "\n\n"))
	flusher.Flush()
	// **The cancel happens BEFORE the count moves, and the order is an observable property.**
	// Incrementing first leaves a window in which the count says a window is here and the grace is
	// still running — so anything that reads the count and then the grace sees a state the server
	// is never actually in. A test caught it at one run in three, and a reader in the field would
	// have seen it far more rarely and had nothing to go on.
	s.cancelIdleExit(idleExitCauseWindow)
	n := s.windows.n.Add(1)
	log.Printf("%s (%d open)", windowConnectedMsg, n)
	defer func() {
		left := s.windows.n.Add(-1)
		log.Printf("%s (%d open)", windowGoneMsg, left)
		if left == 0 {
			s.armIdleExitGrace()
		}
	}()

	// ── The armed state rides this stream (P01.S05, D5) ────────────────────────────
	//
	// **Because `beforeunload` cannot go and ask.** The close prompt must be armed only when
	// something would be lost, and one half of that is "a ceremony is armed or running" — a fact
	// the client had no window-independent way to know: `pollRecv` starts only when THIS window
	// arms, so a window that did not arm never learned the machine was armed, which is exactly the
	// policy-armed ceremony D5 most cares about. `/api/status` carries no such field and is not
	// polled continuously.
	//
	// **Sent here rather than on a new poll, and this is not the heartbeat D1 refused.** D1's
	// objection is to a heartbeat as the WINDOW SIGNAL — a timer cannot tell a minimised window
	// from a closed one. This is state pushed on a socket already held for that signal, on change
	// and never on a clock, so it adds no timer and no connection. The stream's own comment said
	// "nothing is ever sent on this stream"; this is the first thing worth sending.
	//
	// The loop is also the whole handler. net/http cancels the request context when the peer
	// disconnects, so a blocked `select` returns the moment the window is gone — and not before,
	// however long that is. There is no read deadline to survive: the server is constructed with
	// no timeouts (cmd/nib/main.go).
	ctx := r.Context()
	for {
		// **SUBSCRIBE BEFORE READING, and the other order was a lost wakeup (/pending 464).**
		//
		// `armedChangedLocked` broadcasts by CLOSING this channel and setting it to nil, and
		// `armedChanges` lazily makes a fresh one. So obtaining it after the read meant: read the
		// state, marshal, write to the socket, flush — and if the arm landed anywhere in that
		// window, the close-and-nil happened while this loop held no channel at all, and the
		// `armedChanges()` below then handed back a NEW channel that only fires on the NEXT
		// change. The stream parked forever on a change that had already happened.
		//
		// **It is a production defect and not a test flake**, which is how it was found: a window
		// open when a ceremony arms never learns the machine is armed, so `beforeunload` reports
		// nothing-to-lose while a ceremony is running — exactly the policy-armed case D5 exists
		// for, and exactly what this stream was added to carry. It surfaced as
		// `TestTheWindowStreamCarriesTheArmedState` failing under `go test ./...` and passing
		// alone in 0.089s, because the window between the read and the subscribe is a network
		// write and widens under load.
		//
		// Holding the channel first closes it: a change after this line closes a channel this
		// loop already has, and the select returns at once.
		changed := s.sess.armedChanges()
		// **What is armed, not merely whether.** "Names a live ceremony specifically, not
		// generically" (P01.S06) cannot be met from a bool, and the name is the ceremony's own
		// INTENT — the convener's words for what this proceeding is — never a fingerprint, which
		// is the panel's standing rule about naming people.
		armed, what := s.sess.ArmedWhat()
		payload, merr := json.Marshal(armedEvent{Armed: armed, What: what})
		if merr != nil {
			return
		}
		if _, err := w.Write(append(append([]byte("event: armed\ndata: "), payload...), '\n', '\n')); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
	}
}

// handleQuit is the explicit Quit action (P01.S06, D5/D6).
//
// **Guarded by `requirePublicLoopback` and NOT `requireUnlocked`**, which is D3's argument reaching
// one route further: a window sitting on the unlock screen is a real window, and a locked Nib is
// still a Nib its user wants to quit. There is no CSRF token before the vault unlocks, so a
// loopback Origin is the only write guard this route can apply — the same one `/api/handoff` and
// the window stream itself rely on.
//
// **It decides nothing about what would be lost.** The confirmation is the client's, because the
// client is where the user is and where the wording lives (D5); this end is the mechanism. A
// server that second-guessed the answer would be a second place deciding, which is the shape the
// close prompt's one door exists to refuse.
func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	s.RequestExit(exitCauseQuit)
	w.WriteHeader(http.StatusNoContent)
}
