// Package gate bounds how many outside-tool conversions run at once (ADR-185, D9, D31(4), R6).
//
// Eight double-clicked `.docx` files started eight LibreOffices, and a conversion for a window
// that had closed ran on for two minutes. A Gate admits `width` runs; the rest wait in a bounded
// queue and are refused by name past it, and a waiter whose context ends leaves the queue without
// ever holding a slot.
//
// It is a leaf package so that `pdfops` (which owns the converters) and `server` (which owns the
// requests) can both name it without a cycle.
//
// # What the rules are for
//
//   - **The budget starts at acquire**, not at the request: a conversion that waited a minute for
//     its turn is not already a minute into its two (`Run`).
//   - **A slot is released by `defer`**, before any point that can panic, and exactly once.
//   - **A tool called inside never re-acquires.** The gate is taken ONCE at the top of an import; a
//     producer that calls another (an email whose body goes through the browser) would deadlock at
//     width 2. Nothing in this package knows who holds a slot, so the rule is `pdfops`' and its
//     census holds it.
//   - **The queue depth is its own atomic**, read for the "N files waiting" refusal and the window's
//     progress, and it counts only the waiting.
package gate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrQueueFull is a run refused because the queue behind the slots is full. A named refusal: it is
// idempotent (nothing was started, nothing is left behind) and the caller says how many are waiting.
var ErrQueueFull = errors.New("too many conversions are waiting; try fewer files at once")

// Gate admits at most width runs at once and queues at most maxQueue behind them.
type Gate struct {
	slots    chan struct{}
	maxQueue int64
	queued   atomic.Int64
	refused  atomic.Uint64
}

// New makes a Gate. width and maxQueue are at least 1 and 0 respectively.
func New(width, maxQueue int) *Gate {
	if width < 1 {
		width = 1
	}
	if maxQueue < 0 {
		maxQueue = 0
	}
	return &Gate{slots: make(chan struct{}, width), maxQueue: int64(maxQueue)}
}

// Acquire waits for a slot. It returns ErrQueueFull at once when the queue is full, and ctx.Err()
// when the context ends first — in which case no slot is held and the queue depth is restored. The
// returned release is safe to call more than once.
func (g *Gate) Acquire(ctx context.Context) (release func(), err error) { return g.acquire(ctx, true) }

// AcquireWait is Acquire for a caller that must never be refused for a full queue — the OS hand-off,
// where the file manager has already told the person their files are opening (Y4). It still leaves
// when ctx ends, and it still counts in the queue depth.
func (g *Gate) AcquireWait(ctx context.Context) (release func(), err error) {
	return g.acquire(ctx, false)
}

func (g *Gate) acquire(ctx context.Context, bounded bool) (release func(), err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// A free slot is taken without queueing, so the queue bound counts only runs that must wait.
	select {
	case g.slots <- struct{}{}:
		return g.releaser(), nil
	default:
	}
	if g.queued.Add(1) > g.maxQueue && bounded {
		g.queued.Add(-1)
		g.refused.Add(1)
		return nil, ErrQueueFull
	}
	defer g.queued.Add(-1)
	select {
	case g.slots <- struct{}{}:
		return g.releaser(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (g *Gate) releaser() func() {
	var once sync.Once
	return func() { once.Do(func() { <-g.slots }) }
}

// Run acquires a slot, starts the budget THEN, calls admit (if any) and fn on the budgeted context,
// and releases the slot however fn ends. admit is the caller's recheck that a result would still be
// wanted (the document cap): it runs after the wait, because the wait is long enough for the answer to
// change.
func (g *Gate) Run(ctx context.Context, budget time.Duration, admit func(context.Context) error, fn func(context.Context) error) error {
	return g.run(ctx, true, budget, admit, fn)
}

// RunWait is Run through AcquireWait.
func (g *Gate) RunWait(ctx context.Context, budget time.Duration, admit func(context.Context) error, fn func(context.Context) error) error {
	return g.run(ctx, false, budget, admit, fn)
}

func (g *Gate) run(ctx context.Context, bounded bool, budget time.Duration, admit func(context.Context) error, fn func(context.Context) error) error {
	release, err := g.acquire(ctx, bounded)
	if err != nil {
		return err
	}
	defer release()
	bctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if admit != nil {
		if err := admit(bctx); err != nil {
			return err
		}
	}
	return fn(bctx)
}

// Queued is how many runs are waiting for a slot.
func (g *Gate) Queued() int { return int(g.queued.Load()) }

// Held is how many slots are taken.
func (g *Gate) Held() int { return len(g.slots) }

// Width is the number of slots.
func (g *Gate) Width() int { return cap(g.slots) }

// Refused is how many runs ErrQueueFull has turned away.
func (g *Gate) Refused() uint64 { return g.refused.Load() }
