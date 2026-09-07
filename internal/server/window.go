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
)

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
