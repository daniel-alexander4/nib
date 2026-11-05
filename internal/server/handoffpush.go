package server

import (
	"log"
	"sync"
	"sync/atomic"
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
	// Reason is the sentence for a refusal — the import function's own, including the missing-tool one (Y12) —
	// so the window says why instead of "Nib may be full". Empty for every other result.
	Reason string `json:"reason,omitempty"`
	// ImportedFrom is the source's file name when an "opened" hand-off was a conversion; the window says so once.
	ImportedFrom string `json:"importedFrom,omitempty"`
	// Seq is the server's own sequence for the event: two identical outcomes are two events, and the same
	// event delivered twice is one. The page keys an import notice on it (/pending 940).
	Seq uint64 `json:"seq,omitempty"`
}

// handoffNote is one hand-off as the server keeps it. The sequence is the server's own, so two
// identical outcomes are two events.
type handoffNote struct {
	seq uint64
	// held marks a conversion announced while no window was open: the first window to connect is owed it
	// (`claim`), because a document's own answer carries `importedFrom` only on the route that imported it.
	held         bool
	result       string
	reason       string
	importedFrom string
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
func (p *handoffPush) announce(result, reason, importedFrom string) {
	p.mu.Lock()
	p.appendLocked(result, reason, importedFrom, false)
	p.mu.Unlock()
}

// tell is the ONE decision "is a window here to hear it", made under the same lock that counts a window in and out
// (/pending 946 I1, I3): with a window open the outcome is announced, and with none a conversion is held for the first
// that connects — the restore shows its document, not that it was converted. Because arrive and depart take this lock
// too, a window cannot be counted between the read and the hold (the note was delivered by sequence AND stayed held, so
// a reload replayed it), and the count cannot be read as positive for a window that is already gone. Returns the
// windows open.
func (p *handoffPush) tell(n *atomic.Int64, result, reason, importedFrom string) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if open := n.Load(); open > 0 {
		p.appendLocked(result, reason, importedFrom, false)
		return open
	}
	if result == "opened" && importedFrom != "" {
		p.appendLocked(result, reason, importedFrom, true)
	}
	return 0
}

// arrive is a window connecting: under one hold it takes the sequence its stream starts from, collects the conversions
// it is owed (clearing their hold), and counts the window. A window hears the hand-offs that arrive while it is open and
// never the ones before it connected — its boot restore already shows those documents, and replaying a refusal to a
// window opened an hour later would be a notice about nothing. A held conversion is the exception, and it is returned
// APART, to be sent first and exactly once: the start used to move back to just before the oldest, which replayed every
// refusal and focus announced since it too (/pending 946 I2).
func (p *handoffPush) arrive(n *atomic.Int64) (from uint64, owed []handoffNote, count int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	from = p.seq
	for i := range p.recent {
		if p.recent[i].held {
			owed = append(owed, p.recent[i])
			p.recent[i].held = false
		}
	}
	return from, owed, n.Add(1)
}

// depart is a window leaving: it counts the window out, and when it was the last, any conversion announced after the last
// thing it was sent was announced to a stream that will never write it, so it is held again for the next window
// (/pending 946 I3). Returns the windows left.
func (p *handoffPush) depart(n *atomic.Int64, heard uint64) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	left := n.Add(-1)
	if left == 0 {
		for i := range p.recent {
			if ev := &p.recent[i]; ev.seq > heard && ev.result == "opened" && ev.importedFrom != "" {
				ev.held = true
			}
		}
	}
	return left
}

// appendLocked keeps one note and wakes every stream. When the bound is passed the oldest note NOT held goes first: a held
// conversion is owed to a window that has not connected, and dropping it for a refusal that every open window has already
// heard loses the one notice nothing else can replay (/pending 946).
func (p *handoffPush) appendLocked(result, reason, importedFrom string, held bool) uint64 {
	p.seq++
	seq := p.seq
	p.recent = append(p.recent, handoffNote{seq: seq, result: result, reason: reason, importedFrom: importedFrom, held: held})
	for len(p.recent) > handoffKept {
		drop := 0
		for i := range p.recent {
			if !p.recent[i].held {
				drop = i
				break
			}
		}
		p.recent = append(p.recent[:drop], p.recent[drop+1:]...)
	}
	if p.watch != nil {
		close(p.watch)
		p.watch = nil
	}
	return seq
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

// tellWindows is the ONE door an outcome reaches the windows through (ADR-009): the open windows hear it by
// sequence, and a conversion with no window open is held for the first that connects. Returns the windows open.
func (s *Server) tellWindows(result, reason, importedFrom string) int64 {
	return s.push.tell(&s.windows.n, result, reason, importedFrom)
}

// windowHas decides whether the launch behind a hand-off should open a window, announces the
// hand-off to the windows that are open, and reports the answer as the route's `surfaced`.
//
// **A launch with no document always gets a window.** It is how a page that never held the session
// gets in — the launch overlay says so in as many words — so it cannot be answered by a window the
// user may not be looking at. **A refusal needs a window that is actually open**, because the
// refusal is a sentence and a window still on its way was never told it.
func (s *Server) windowHas(result, reason, importedFrom string) (surfaced bool) {
	if result == "window" {
		s.push.expect()
		return false
	}
	n := s.tellWindows(result, reason, importedFrom)
	switch {
	case n > 0:
		log.Printf("%s %s (%d open)", handoffTakenMsg, result, n)
		return true
	case result != "refused" && s.push.expected():
		log.Printf("%s %s", handoffAwaitsItsOwnW, result)
		return true
	}
	s.push.expect()
	return false
}
