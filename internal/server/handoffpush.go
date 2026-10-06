package server

import (
	"log"
	"sync"
	"time"
)

// A launch into a running Nib is a tab in the window that is already open (ADR-086).
//
// A hand-off installs its document in this process, and the window that was already open used to
// have no way to hear of it: the stream carried the armed state and the download and nothing else,
// so the only thing that showed the document was a SECOND window, restoring everything. The
// hand-off is now pushed on the stream each window already holds, and the launch is told whether a
// window has it — so it opens one only when none does.

// handoffEvent is one hand-off as a window hears it: which of the route's outcomes.
type handoffEvent struct {
	Result string `json:"result"` // opened | focused | queued | refused
}

// handoffNote is one hand-off as the server keeps it. The sequence is the server's own, so two
// identical outcomes are two events; the page has no use for it and it never reaches the wire.
type handoffNote struct {
	seq    uint64
	result string
}

// handoffKept bounds the events held for a stream that is mid-write when several arrive. It is the
// document cap because a launch per selected file is the burst that exists.
const handoffKept = maxOpenDocs

// windowExpectedFor is how long a window that was asked for is waited on before a launch opens
// another. Measured 2026-10-06 (this machine, Chrome app mode, a FRESH profile, three runs): 7.5 s
// to 10.0 s from "serving" to the stream connecting. Three times the slowest.
const windowExpectedFor = 30 * time.Second

const (
	handoffTakenMsg      = "hand-off taken by an open window:"
	windowNotArrivedMsg  = "expected window did not arrive"
	handoffAwaitsItsOwnW = "hand-off waits for the window already on its way:"
)

type handoffPush struct {
	mu     sync.Mutex
	seq    uint64
	recent []handoffNote
	watch  chan struct{}
	// expectUntil is non-zero while a window has been asked for and has not connected.
	expectUntil time.Time
}

// announce records one hand-off and wakes every window's stream.
func (p *handoffPush) announce(result string) {
	p.mu.Lock()
	p.seq++
	p.recent = append(p.recent, handoffNote{seq: p.seq, result: result})
	if len(p.recent) > handoffKept {
		p.recent = p.recent[len(p.recent)-handoffKept:]
	}
	if p.watch != nil {
		close(p.watch)
		p.watch = nil
	}
	p.mu.Unlock()
}

// changes returns a channel closed on the next hand-off. Same contract as armedChanges: take it
// BEFORE reading, or a hand-off landing in between is lost (/pending 464).
func (p *handoffPush) changes() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.watch == nil {
		p.watch = make(chan struct{})
	}
	return p.watch
}

// current is the sequence a stream starts from. A window hears the hand-offs that arrive while it
// is open and never the ones before it connected: its boot restore already shows those documents,
// and replaying a refusal to a window opened an hour later would be a notice about nothing.
func (p *handoffPush) current() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.seq
}

// since returns the hand-offs after seq, oldest first.
func (p *handoffPush) since(seq uint64) []handoffNote {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []handoffNote
	for _, ev := range p.recent {
		if ev.seq > seq {
			out = append(out, ev)
		}
	}
	return out
}

// expect records that a window has been asked for: by this process's own launch, or by a hand-off
// answered "open one".
func (p *handoffPush) expect() {
	p.mu.Lock()
	p.expectUntil = time.Now().Add(windowExpectedFor)
	p.mu.Unlock()
}

// arrived clears the expectation: the window came, or nothing could launch one.
func (p *handoffPush) arrived() {
	p.mu.Lock()
	p.expectUntil = time.Time{}
	p.mu.Unlock()
}

// expected reports whether a window asked for is still on its way. One that never came is logged
// once and forgotten, so the launch asking now opens its own.
func (p *handoffPush) expected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.expectUntil.IsZero() {
		return false
	}
	if time.Now().After(p.expectUntil) {
		p.expectUntil = time.Time{}
		log.Printf("%s within %s; the next launch opens one", windowNotArrivedMsg, windowExpectedFor)
		return false
	}
	return true
}

// ExpectWindow tells the server this process is about to open its own window, so a launch arriving
// before that window connects does not open a second (ADR-086).
func (s *Server) ExpectWindow() { s.push.expect() }

// ExpectNoWindow withdraws it: nothing could be launched.
func (s *Server) ExpectNoWindow() { s.push.arrived() }

// windowHas decides whether the launch behind a hand-off should open a window, announces the
// hand-off to the windows that are open, and reports the answer as the route's `surfaced`.
//
// **A launch with no document always gets a window.** It is how a page that never held the session
// gets in — the launch overlay says so in as many words — so it cannot be answered by a window the
// user may not be looking at. **A refusal needs a window that is actually open**, because the
// refusal is a sentence and a window still on its way was never told it.
func (s *Server) windowHas(result string) (surfaced bool) {
	if result == "window" {
		s.push.expect()
		return false
	}
	n := s.windows.n.Load()
	switch {
	case n > 0:
		s.push.announce(result)
		log.Printf("%s %s (%d open)", handoffTakenMsg, result, n)
		return true
	case result != "refused" && s.push.expected():
		log.Printf("%s %s", handoffAwaitsItsOwnW, result)
		return true
	}
	s.push.expect()
	return false
}
