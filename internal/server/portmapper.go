package server

import (
	"context"
	"log"
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
	// SetOnRequestSent installs the send-time recorder. In the interface rather than set by
	// the caller on the concrete type, so production and the fake wire it through the SAME
	// door — a recorder the test installs and production forgets is the fixture supplying
	// what production omits, which is how a green here would mean nothing.
	SetOnRequestSent(fn func(portmap.Mapping))
	// ObserveLease resolves a lease the mechanism could not report at obtain time. In the
	// interface for the same reason SetOnRequestSent is: production and the fake go through
	// the same door, so a green here cannot come from a test-only hookup.
	ObserveLease(ctx context.Context, m portmap.Mapping) (portmap.Mapping, error)
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

	mu      sync.Mutex
	current portmap.Mapping
	have    bool
	// pending are handles for requests that LEFT this host and may therefore have created a
	// mapping, whether or not a reply ever came back. `current` is what we know we have;
	// this is what we might have, and close() deletes both (/pending 257).
	pending   []portmap.Mapping
	closed    bool
	started   bool
	portMoved bool // a refresh got a DIFFERENT external port — item 20's stale-record case
	reported  bool // reportLease has already spoken for this mapper

	stop chan struct{}
	done chan struct{}
}

func newPortMapper(client portMapClient, proto portmap.Protocol, internalPort uint16) *portMapper {
	p := &portMapper{
		client: client, proto: proto, internalPort: internalPort,
		refreshFloor: 15 * time.Second,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
	client.SetOnRequestSent(p.recordPending)
	return p
}

// maxPendingHandles bounds the set. One obtain plus a refresh every cycle over a long arm is the
// only way it grows, and each entry is a handle we may have to delete; a cap keeps a wedged
// router from turning teardown into an unbounded delete storm.
const maxPendingHandles = 8

// recordPending is the send-time recorder. It is called from inside the portmap client, on
// whatever goroutine made the request.
func (p *portMapper) recordPending(m portmap.Mapping) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.pending {
		if e == m {
			return // the same request, retransmitted or repeated by a refresh
		}
	}
	if len(p.pending) >= maxPendingHandles {
		return
	}
	p.pending = append(p.pending, m)
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

// refreshAfter is the refresh cadence: half the granted lease, floored, and deliberately with
// NO ceiling.
//
// **No ceiling, and the reason is that a ceiling cannot do the job it looks like it does.**
// /pending 253 asked for the granted lease to be clamped so D15's crash guarantee ("a mapping
// left by a SIGKILLed Nib expires on its own") holds independent of the router. It cannot:
// clamping this number changes a local variable, while the lease the ROUTER holds is whatever
// it granted, and after SIGKILL nothing of ours runs to shorten it. Re-requesting is not a
// mechanism either — RFC 6887 and RFC 6886 both let the server assign what it likes, and Refresh
// already re-sends 120 s every cycle. So the honest position is that the crash floor is the
// router's, recorded where a reader will meet it, and the only real bound would be a delete on
// NEXT START, which puts a router port and a control URL on disk and wants its own decision.
// A ceiling would also cost round trips for nothing: 7200 s granted gives a 3600 s wait against
// a 300 s race, so the timer never fires, and close() deletes the mapping whatever it says.
//
// **The live defect on this line was the FLOOR, in the opposite direction from the item.** An
// unconditional floor of 15 s applied to a lease SHORTER than itself refreshes after the mapping
// has already expired — the published record names a dead port until the next cycle, and nothing
// detects it. So the floor binds only while it is still inside the lease.
func refreshAfter(lifetimeSec uint32, floor time.Duration) time.Duration {
	if lifetimeSec == 0 {
		// No grant was reported at all: the floor is the only cadence there is.
		//
		// **This used to say "the UPnP path", and that was wrong the day it was written**
		// (/pending 260). The UPnP branch sets LifetimeSec to the 120 s it REQUESTED and marks
		// it unobserved, so it never reaches here — it lands on the ordinary half-the-lease
		// path. What reaches here is a socket-protocol router reporting a zero lifetime.
		// The distinction matters beyond the comment: it is why an observed-permanent lease is
		// carried in LeasePermanent rather than as a zero LifetimeSec, which this branch would
		// read as its opposite and refresh every floor-interval forever.
		return floor
	}
	lease := time.Duration(lifetimeSec) * time.Second
	wait := lease / 2
	if wait < floor && floor < lease {
		wait = floor
	}
	return wait
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
			wait := refreshAfter(m.LifetimeSec, p.refreshFloor)
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

			// Resolve a lease the obtain could not report, before refreshing on it. Driven by
			// the PREDICATE rather than by a tick count: `p.current = nm` below replaces the
			// mapping after every successful refresh, and the UPnP path returns a freshly
			// unobserved one each time, so a one-shot observation would be erased by the very
			// next cycle. Its own context, off the refresh's budget, which SSDP already
			// nearly exhausts.
			if !cur.LifetimeObserved {
				octx, ocancel := context.WithTimeout(context.Background(), portMapBudget)
				observed, oerr := p.client.ObserveLease(octx, cur)
				ocancel()
				if oerr == nil && observed.LifetimeObserved {
					p.reportLease(observed)
					cur = observed
					p.mu.Lock()
					if !p.closed {
						p.current = observed
					}
					p.mu.Unlock()
				}
			}
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
			// **Carry the observation across the refresh.** UPnP's Refresh re-runs the obtain,
			// which reports the lease we ASKED for and marks it unobserved — so storing `nm`
			// as-is threw away what ObserveLease had just learned, flipped the mapping back to
			// unobserved every cycle, and made the loop re-read the lease (and re-log a
			// permanent one) on every tick forever. Only carried when the refresh names the
			// same mapping; a moved port is a different mapping and its lease is unknown again.
			if cur.LifetimeObserved && nm.SameTarget(cur) {
				nm.LifetimeObserved, nm.LeasePermanent = true, cur.LeasePermanent
				nm.LifetimeSec = cur.LifetimeSec
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

// reportLease is the READER, and without one this whole path is a third writer to a field
// nothing consumes.
//
// /pending 258 — delete a mapping a crashed run left behind — is deferred behind a gate that
// reads "a permanent or over-hour lease actually OBSERVED on the UPnP path". Nothing could
// observe one, and a value resolved into a struct that nobody prints does not discharge it
// either: an observation nobody can read is not evidence. Nib has no telemetry and this is not
// a connect failure, so the log is the channel — the local-first SRE's only one.
//
// Silent for the ordinary case. It fires for exactly the two states the gate names.
func (p *portMapper) reportLease(m portmap.Mapping) {
	p.mu.Lock()
	if p.reported {
		p.mu.Unlock()
		return // once per mapper: the fact does not change, and a log line per tick is noise
	}
	p.reported = true
	p.mu.Unlock()
	switch {
	case m.LeasePermanent:
		log.Printf("port mapping on external port %d is PERMANENT — the router ignored the %ds lease "+
			"we asked for, so a mapping left by a crash will not expire on its own (/pending 258)",
			m.ExternalPort, portmap.DefaultLeaseSec)
	case m.LifetimeSec > 3600:
		log.Printf("port mapping on external port %d was granted a %ds lease against the %ds we asked "+
			"for — a mapping left by a crash outlives the process by that long (/pending 258)",
			m.ExternalPort, m.LifetimeSec, portmap.DefaultLeaseSec)
	}
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
	pending := append([]portmap.Mapping(nil), p.pending...)
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
	// FRESH context (C2): the arm context is cancelled by the teardown that called us.
	ctx, cancel := context.WithTimeout(context.Background(), portMapBudget)
	defer cancel()
	if have {
		_ = p.client.Unmap(ctx, m)
	}
	// And everything we may have created but never got an answer for. Deleting a mapping that
	// was never made is a no-op on all three mechanisms — the socket protocols never read a
	// reply, and a UPnP delete of a nonexistent entry returns an error this discards — so the
	// safe direction is to send it. Without this, a Map that failed AFTER its request went out
	// left a mapping alive to lease expiry with nothing holding a handle to it.
	for _, pm := range pending {
		// The confirmed mapping supersedes the handle recorded for the request that produced
		// it — same mechanism, protocol and internal port is the same thing to delete, and the
		// granted external port and lease that make the two structs differ are fields a delete
		// never uses. Comparing whole structs here would send two deletes for one mapping.
		if have && pm.SameTarget(m) {
			continue
		}
		_ = p.client.Unmap(ctx, pm)
	}
}
