package p2p

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"nib/internal/safe"
)

// listenerCore is the termination protocol both listeners run, written once.
//
// # Why this exists
//
// `tlsListener` and `quicListener` carried this protocol in two near-verbatim copies of
// about seventy-five lines: `once`/`start`, a `ready` that is buffered and never closed,
// a `sem` bounding concurrent handshakes, a `done` closed before the underlying listener,
// `mu`/`closed`/`cerr`, `setCloseErr`, a `closeErr` that always wraps `net.ErrClosed`, an
// `Accept` selecting on the two channels, and a drain. P05.S02 produced the second copy by
// copying the first.
//
// **ADR-009 is the law it sits under** — a rule holding at more than one call site is
// written once and every site calls it — and this is a whole protocol, not a rule. It is
// also not the platform split ADR-009 declines to collapse: build-tagged siblings are
// correct where the API differs, and these two are one API with two transports beneath it.
//
// # Where the seam is
//
// Everything above the accept loop is shared and everything in it is not. What differs
// between TCP and QUIC is (a) how a raw connection is accepted and turned into a `*Conn`,
// and (b) what sits underneath this protocol and must be taken down with it. Both are
// supplied as functions by the listener that embeds this, so the core never learns which
// transport it is running — the same discipline `transports_test.go` enforces on the tests.
type listenerCore struct {
	// loop is the transport's accept loop and shut is what it takes down beneath this
	// protocol. Both are assigned by the constructor immediately after the listener is
	// built, because both close over the listener that embeds this core.
	loop func()
	shut func() error

	once  sync.Once
	ready chan *Conn    // handshakes that completed; NEVER closed — see the note below
	done  chan struct{} // closed by Close, the single termination signal
	sem   chan struct{} // bounds concurrent handshakes

	mu     sync.Mutex
	closed bool
	cerr   error
}

// maxConcurrentHandshakes bounds the in-flight handshakes.
//
// **The kernel backlog used to be this bound and moving the handshake off the accept path
// removed it.** Unbounded, one goroutine and one fd per inbound connection for the whole
// 30 s handshakeTimeout is an fd exhaustion any host on the segment can drive: the listener
// binds 0.0.0.0:0 and broadcasts its port every 500 ms, and each connection costs the
// attacker one SYN. A GUI process on macOS has 256 descriptors by default.
//
// Sixteen, and the acquire BLOCKS rather than dropping: a full pool stalls the accept loop,
// which pushes the excess back into the kernel backlog — where it was before, and where it
// costs Nib nothing. The residual is that sixteen simultaneous stalls delay the genuine peer
// by up to one handshakeTimeout, which is strictly better than the one-stall-blocks-everything
// this replaced and than the fd exhaustion the unbounded version allowed.
const maxConcurrentHandshakes = 16

// newListenerCore builds the channels.
//
// **Built here and not in start(), so Close can read them without racing the first
// Accept.** They are the listener's, not the accept loop's.
func newListenerCore() listenerCore {
	return listenerCore{
		done:  make(chan struct{}),
		ready: make(chan *Conn, maxConcurrentHandshakes),
		sem:   make(chan struct{}, maxConcurrentHandshakes),
	}
}

// Close stops the accept loop and releases anything blocked in Accept.
//
// `done` is closed BEFORE `shut` runs, so a handshake finishing at any point after it is
// closed by `offer` instead of leaking.
// Idempotent, because a session teardown and an explicit disarm can both reach it — the
// same reason the announcer's Close is.
func (l *listenerCore) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	if l.cerr == nil {
		l.cerr = net.ErrClosed
	}
	l.mu.Unlock()
	close(l.done)
	err := l.shut()
	// Drain what the handshake goroutines queued. `ready` is buffered since P05.S02, so
	// a completed connection can be sitting in it when the session ends; nothing else
	// will ever take it. A handshake that completes AFTER this drain is `offer`'s to close:
	// it re-checks `done` after its send, and `done` is closed before this line runs.
	l.drain()
	return err
}

// drain closes every connection queued in `ready`, without blocking.
func (l *listenerCore) drain() {
	for {
		select {
		case c := <-l.ready:
			c.Close()
		default:
			return
		}
	}
}

// offer hands a completed handshake to the accept loop — the ONE hand-off both transports'
// handshake goroutines use (ADR-009).
//
// **After `Close`, nothing queued here is ever taken, and `select` does not prefer `done`.** The
// two copies this replaced selected between the buffered send and `<-l.done`; once Close has run,
// `done` is closed AND `ready` has room, and Go picks uniformly between ready cases — so a
// handshake finishing any time after Close's drain queued its connection with probability 1/2
// and leaked it, not only one finishing "in the same instant" as the comment there claimed (the
// returned-document P02 phase-close review, /pending 807 R7). A handshake can finish up to
// `handshakeTimeout` after Close, because `shut` closes only the listener beneath.
//
// So `done` is asked first, and asked AGAIN after a successful send: Close closes `done` before it
// drains, so a send that landed after that drain is seen here and drained by this goroutine. Every
// interleaving ends with the connection closed or with an accept loop that is still live.
func (l *listenerCore) offer(conn *Conn) {
	select {
	case <-l.done:
		conn.Close()
		return
	default:
	}
	select {
	case l.ready <- conn:
		// Buffered, so this does not block and the slot is released at once.
		select {
		case <-l.done:
			l.drain()
		default:
		}
	default:
		// The queue is full: `maxConcurrentHandshakes` connections are already
		// waiting for a serial accept loop that serves one session. This one is
		// not going to be served, and holding it would cost the slot that lets
		// the NEXT peer handshake at all.
		//
		// **What is dropped here is a PINNED peer's connection**, not a stranger's
		// — nothing else can reach this line, because a failed handshake returned
		// above. That sounds worse than it is and the argument is worth stating:
		// the only thing that can fill this queue is a peer connecting far faster
		// than one session can be served, which is our own racer (P05.S03) and is
		// bounded well below the buffer. A genuine peer that is dropped sees a
		// closed connection and redials, against a listener that is still armed.
		conn.Close()
	}
}

// setCloseErr records why the loop stopped, first cause winning.
func (l *listenerCore) setCloseErr(err error) {
	l.mu.Lock()
	if l.cerr == nil {
		l.cerr = err
	}
	l.mu.Unlock()
}

// closeErr is the error Accept returns once the listener is finished.
//
// **It always wraps net.ErrClosed, and the wrap is the contract.** `Listener.Accept`'s doc
// says *"A closed listener reports net.ErrClosed, which is how the loop ends"*, and
// `runSession` exits only on `errors.Is(err, net.ErrClosed)`. Returning a bare accept error —
// an EMFILE from fd exhaustion, say — made that loop spin at 100 % of a core with no syscall,
// and `Close` could not rescue it because `cerr` was already set, so the session never
// disarmed and its announcer broadcast for the life of the process.
//
// The cause is kept alongside so a diagnostic can still say *why* it stopped.
func (l *listenerCore) closeErr() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cerr != nil && !errors.Is(l.cerr, net.ErrClosed) {
		return fmt.Errorf("%w: %v", net.ErrClosed, l.cerr)
	}
	return net.ErrClosed
}

// Accept returns the next connection that COMPLETED a pinned handshake.
//
// # The handshake runs off the accept path, and that is the fix
//
// It used to run inline: accept, then handshake, in the same call — so one connection that
// opened and sent nothing held the caller for the whole `handshakeTimeout`. The server's arm
// loop is a single goroutine calling this, and the listener's port is broadcast to the
// multicast group every 500 ms, so ten such connections consumed the entire five-minute arm
// window and the genuine peer was never accepted. No scan required.
//
// `internal/server/l1_test.go` states the property verbatim — "measured at 30 s per stalled
// connection, and filed as its own item" — and the guard that existed at the time,
// `TestAStrayConnectionDoesNotConsumeTheSession`, only proved the session SURVIVES one stray
// connection. It never measured that the loop was blocked, so the fix for the
// one-shot-consumption defect left the head-of-line block in place and untested.
//
// Now a single goroutine accepts and hands each raw connection to its own handshaker; only
// completed ones reach the channel. A stalled peer costs one goroutine and one timeout,
// concurrently with every other, and cannot delay the real one.
func (l *listenerCore) Accept() (*Conn, error) {
	l.start()
	select {
	case c := <-l.ready:
		return c, nil
	case <-l.done:
		return nil, l.closeErr()
	}
}

// start launches the accept loop once.
//
// The recover is HERE rather than inside each transport's acceptLoop, and that is ADR-009:
// one launch site covers both transports, where two copies in two bodies is a rule with two
// doors and a third transport arriving with neither. `loop` is a func FIELD, so a bare
// `go l.loop()` also has no declaration for the guard to resolve it through — the wrapper is
// what makes the coverage visible as well as true.
//
// **Every exit of the loop closes the listener, a panic included** (/pending 501). Both loops end
// through `Close` on their own error paths, but a panic unwound past those calls: `safe.Recover`
// kept the process alive and nothing ever closed `done`, so `Accept` blocked until somebody else
// closed the listener — the whole arm window — on a listener that could no longer accept anything.
// The deferred close runs before the recover, while the panic unwinds. `setCloseErr` is
// first-cause-wins and `Close` is idempotent, so on an ordinary exit both are no-ops.
func (l *listenerCore) start() {
	l.once.Do(func() {
		go func() {
			defer safe.Recover("accept loop")
			defer func() {
				l.setCloseErr(errAcceptLoopEnded)
				_ = l.Close()
			}()
			l.loop()
		}()
	})
}

// errAcceptLoopEnded is the cause recorded when the accept loop ended without saying why — which
// only a panic does, since both loops record their own cause first.
var errAcceptLoopEnded = errors.New("the accept loop ended")
