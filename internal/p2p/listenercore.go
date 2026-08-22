package p2p

import (
	"errors"
	"fmt"
	"net"
	"sync"
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
// `done` is closed BEFORE `shut` runs, so a handshake finishing in the same instant offers
// into a channel nobody is reading and closes the connection instead of leaking it.
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
	// will ever take it.
	//
	// **The residual is declared rather than closed**: a goroutine that wins the race
	// between `<-l.done` and the buffered send in the same instant as this drain leaves
	// one connection queued. It is bounded by the buffer, it costs one fd until the
	// process reclaims it, and closing it deterministically would mean blocking Close
	// until every handshake goroutine has returned — on a path the user reaches by
	// pressing Cancel.
	for {
		select {
		case c := <-l.ready:
			c.Close()
		default:
			return err
		}
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
func (l *listenerCore) start() {
	l.once.Do(func() { go l.loop() })
}
