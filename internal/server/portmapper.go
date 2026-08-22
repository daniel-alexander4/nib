package server

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"nib/internal/portmap"
	"nib/internal/safe"
)

// portMapClient is the slice of *portmap.Client the mapper needs — an interface so the refresh
// loop is driven by a fake in tests, never a real socket or router.
type portMapClient interface {
	Map(ctx context.Context, proto portmap.Protocol, internalPort uint16) (portmap.Mapping, netip.Addr, error)
	Refresh(ctx context.Context, m portmap.Mapping) (portmap.Mapping, netip.Addr, error)
	Unmap(ctx context.Context, m portmap.Mapping) error
}

// portMapper manages ONE router mapping over a ceremony's life (P05.S07): obtained once at
// publish, refreshed while armed, deleted on teardown.
//
// The whole point of the type is the grill's concurrency findings:
//   - the refresh runs on the ARM context, not the publish budget (C4), or it dies mid-race;
//   - `close()` marks closed, JOINS the refresh goroutine, THEN deletes — cancel is not join,
//     and an in-flight refresh landing after the delete would re-create the mapping (C3, the
//     P05.S03 leak shape);
//   - the delete runs on a FRESH context (C2): the arm context is cancelled by teardown, so a
//     delete derived from it is an instant no-op.
type portMapper struct {
	client       portMapClient
	proto        portmap.Protocol
	internalPort uint16
	refreshFloor time.Duration // smallest refresh interval; a field so tests can shrink it

	mu        sync.Mutex
	current   portmap.Mapping
	have      bool
	closed    bool
	started   bool
	portMoved bool // a refresh got a DIFFERENT external port — item 20's stale-record case

	stop chan struct{}
	done chan struct{}
}

func newPortMapper(client portMapClient, proto portmap.Protocol, internalPort uint16) *portMapper {
	return &portMapper{
		client: client, proto: proto, internalPort: internalPort,
		refreshFloor: 15 * time.Second,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// obtain performs the first mapping synchronously and returns the external address to publish.
// ctx bounds only this call (the arm's 3 s portMapBudget); it is NOT the refresh's context.
func (p *portMapper) obtain(ctx context.Context) (netip.AddrPort, bool) {
	m, ext, err := p.client.Map(ctx, p.proto, p.internalPort)
	if err != nil {
		return netip.AddrPort{}, false
	}
	p.mu.Lock()
	p.current, p.have = m, true
	p.mu.Unlock()
	return netip.AddrPortFrom(ext, m.ExternalPort), true
}

// startRefresh spawns the refresh loop. It is **self-contained**: stopped only by close()'s
// `stop` channel, and its Refresh calls use a FRESH bounded context — so it is decoupled from
// the publish goroutine's context, whose `defer cancel()` fires the instant publish returns
// (diff-grill #1). Binding the refresh to that context killed it seconds after it started, the
// exact opposite of "refresh while armed". Idempotent-safe against a close() that already fired.
func (p *portMapper) startRefresh() {
	p.mu.Lock()
	if p.closed || p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.mu.Unlock()

	go func() {
		defer safe.Recover("port-map refresh")
		defer close(p.done)
		for {
			p.mu.Lock()
			m, have, closed := p.current, p.have, p.closed
			p.mu.Unlock()
			if closed || !have {
				return
			}
			wait := time.Duration(m.LifetimeSec/2) * time.Second
			if wait < p.refreshFloor {
				wait = p.refreshFloor
			}
			timer := time.NewTimer(wait)
			select {
			case <-p.stop:
				timer.Stop()
				return
			case <-timer.C:
			}
			// Re-check closed under the lock BEFORE the network call — no refresh after close
			// begins (C3); close() then joins us before deleting, so no resurrection.
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				return
			}
			cur := p.current
			p.mu.Unlock()

			rctx, cancel := context.WithTimeout(context.Background(), portMapBudget)
			nm, _, err := p.client.Refresh(rctx, cur)
			cancel()
			if err != nil {
				continue // a failed refresh is not fatal; the lease still has time — try next cycle
			}
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				return
			}
			if nm.ExternalPort != p.current.ExternalPort {
				// The router did not honour the same-port request: the published record now
				// names a dead port. Re-publishing under a new port is item 20 (the
				// CandidateGate cap), phase-blocked — detected here, not silently continued.
				p.portMoved = true
			}
			p.current = nm
			p.mu.Unlock()
		}
	}()
}

// close stops the refresh, JOINS the goroutine, and deletes the mapping on a FRESH context.
// Idempotent. Bounded so a wedged refresh or a slow IGD delete cannot hang teardown.
func (p *portMapper) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	m, have, started := p.current, p.have, p.started
	p.mu.Unlock()

	close(p.stop)
	if started {
		select {
		case <-p.done: // the refresh goroutine has exited; no refresh can now race the delete
		case <-time.After(3 * time.Second):
			// A wedged goroutine must not hang the user's Cancel; the delete below still runs,
			// and the lease is the backstop for anything the wedge left behind.
		}
	}
	if have {
		// FRESH context (C2): the arm context is cancelled by the teardown that called us.
		ctx, cancel := context.WithTimeout(context.Background(), portMapBudget)
		defer cancel()
		_ = p.client.Unmap(ctx, m)
	}
}
