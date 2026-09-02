package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"log"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nib/internal/portmap"
)

// fakeMapClient records calls and lets a test drive the mapper without a socket or router.
type fakeMapClient struct {
	extIP     netip.Addr
	extPort   uint16 // the external port granted; a refresh may return a DIFFERENT one if moveTo != 0
	moveTo    uint16
	maps      int32
	refreshes int32
	unmaps    int32

	gate     chan struct{} // if non-nil, Refresh blocks receiving from it
	inflight chan struct{} // if non-nil, Refresh signals here when it has started

	// onSent is the send-time recorder, installed through the SAME interface method production
	// uses (newPortMapper wires it), so a green here cannot come from a test-only hookup.
	onSent   func(portmap.Mapping)
	observes int32
	// observed, when set, is what ObserveLease resolves the lease to — the fixture for a router
	// whose real lease is not the one we asked for.
	observed *portmap.Mapping
	// failAfterSending makes Map behave like a request that LEFT the host and then failed —
	// the /pending 257 case: the router may have created a mapping, and Map returns an error.
	failAfterSending bool

	mu                sync.Mutex
	unmapped          bool
	refreshAfterUnmap bool // set if a Refresh is ever seen after an Unmap — the C3 resurrection
}

func (f *fakeMapClient) SetOnRequestSent(fn func(portmap.Mapping)) { f.onSent = fn }

// ObserveLease stands in for the UPnP read-back. `observed` is what the router turns out to have
// granted; leaving it zero-valued means "the mechanism reports its own lease", which is every
// mechanism but UPnP and is the default the existing rows want.
func (f *fakeMapClient) ObserveLease(ctx context.Context, m portmap.Mapping) (portmap.Mapping, error) {
	atomic.AddInt32(&f.observes, 1)
	if f.observed == nil {
		return m, nil
	}
	return *f.observed, nil
}

func (f *fakeMapClient) Map(ctx context.Context, proto portmap.Protocol, internalPort uint16) (portmap.Mapping, netip.Addr, error) {
	atomic.AddInt32(&f.maps, 1)
	if f.onSent != nil {
		f.onSent(portmap.Mapping{Protocol: proto, InternalPort: internalPort})
	}
	if f.failAfterSending {
		return portmap.Mapping{}, netip.Addr{}, errors.New("portmap: no response")
	}
	// LifetimeSec 0 / LifetimeObserved false: "no grant reported", which is both the honest
	// UPnP value and the one case where refreshAfter returns the floor unchanged — so the test
	// governs the cadence via refreshFloor rather than through a real 1-second interval that
	// would race a 2 s deadline.
	return portmap.Mapping{Protocol: proto, InternalPort: internalPort, ExternalPort: f.extPort}, f.extIP, nil
}

func (f *fakeMapClient) Refresh(ctx context.Context, m portmap.Mapping) (portmap.Mapping, netip.Addr, error) {
	atomic.AddInt32(&f.refreshes, 1)
	if f.inflight != nil {
		select {
		case f.inflight <- struct{}{}:
		default:
		}
	}
	if f.gate != nil {
		<-f.gate // block until the test releases us, mid-call
	}
	f.mu.Lock()
	if f.unmapped {
		f.refreshAfterUnmap = true // a refresh landed AFTER the delete — the leak the grill's C3 forbids
	}
	f.mu.Unlock()
	ext := m.ExternalPort
	if f.moveTo != 0 {
		ext = f.moveTo
	}
	nm := m
	nm.ExternalPort = ext
	// **A refresh comes back UNOBSERVED, because that is what the real one does.** UPnP's
	// Refresh re-runs the obtain, and IGD's AddPortMapping has no lease out-argument, so the
	// mapping it returns carries the lease we ASKED for with LifetimeObserved false. Echoing
	// the input mapping instead made the fake preserve an observation production discards —
	// the fixture supplying what production omits, which made the carry-forward row vacuous.
	nm.LifetimeObserved, nm.LeasePermanent = false, false
	return nm, f.extIP, nil
}

func (f *fakeMapClient) Unmap(ctx context.Context, m portmap.Mapping) error {
	atomic.AddInt32(&f.unmaps, 1)
	f.mu.Lock()
	f.unmapped = true
	f.mu.Unlock()
	return nil
}

func newFake() *fakeMapClient {
	return &fakeMapClient{extIP: netip.MustParseAddr("9.9.9.9"), extPort: 51000}
}

// The happy path: obtain returns the external address, refresh fires while armed, and close
// deletes exactly once and joins the goroutine so nothing refreshes afterward.
func TestPortMapperObtainRefreshDelete(t *testing.T) {
	f := newFake()
	pm := newPortMapper(f, portmap.UDP, 40404)
	pm.refreshFloor = 20 * time.Millisecond // so the test does not wait 15s

	ap, ok := pm.obtain(context.Background())
	if !ok {
		t.Fatal("obtain failed")
	}
	if ap.Addr() != f.extIP || ap.Port() != f.extPort {
		t.Fatalf("obtain returned %v, want %v:%d", ap, f.extIP, f.extPort)
	}

	pm.startRefresh()

	// Let a couple of refreshes fire.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&f.refreshes) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&f.refreshes) < 2 {
		t.Fatalf("refresh fired %d times, want >= 2 — the mapping is not being kept alive", f.refreshes)
	}

	pm.close()

	if atomic.LoadInt32(&f.unmaps) != 1 {
		t.Errorf("Unmap called %d times, want exactly 1", f.unmaps)
	}
	// The join guarantee: give any stray refresh a moment, then assert none landed after Unmap.
	time.Sleep(100 * time.Millisecond)
	f.mu.Lock()
	if f.refreshAfterUnmap {
		t.Error("a refresh landed AFTER the delete — close() did not join the goroutine, so the mapping was re-created (grill C3)")
	}
	f.mu.Unlock()
}

// close() is idempotent and safe to call with no refresh ever started (the unroutable-endpoint
// path obtains but does not start the refresh).
func TestPortMapperCloseIsIdempotentAndSafeWithoutRefresh(t *testing.T) {
	f := newFake()
	pm := newPortMapper(f, portmap.UDP, 40404)
	if _, ok := pm.obtain(context.Background()); !ok {
		t.Fatal("obtain failed")
	}
	// No startRefresh — as when the mapped address failed the addrscope screen.
	pm.close()
	pm.close() // second close must not panic (double close of stop) or double-Unmap
	if got := atomic.LoadInt32(&f.unmaps); got != 1 {
		t.Errorf("Unmap called %d times across two closes, want exactly 1", got)
	}
}

// A refresh that gets a DIFFERENT external port sets portMoved (item 20), rather than silently
// continuing to refresh a port the published record no longer names.

func TestPortMapperFlagsAMovedExternalPort(t *testing.T) {
	f := newFake()
	f.moveTo = 52999 // the router will assign a different port on refresh
	pm := newPortMapper(f, portmap.UDP, 40404)
	pm.refreshFloor = 20 * time.Millisecond
	pm.obtain(context.Background())
	pm.startRefresh()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pm.mu.Lock()
		moved := pm.portMoved
		pm.mu.Unlock()
		if moved {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	pm.close()
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if !pm.portMoved {
		t.Error("the router changed the external port on refresh and portMoved was not set — item 20's stale record goes undetected")
	}
}

// The join, proved robustly (diff-grill #2): a refresh is FORCED in-flight across close(). If
// close() did not wait for the goroutine to finish (the `<-done` join), Unmap would run while
// the refresh is still executing, and the refresh — resuming after the gate opens — would see
// `unmapped` and flag the resurrection. With the join, close() blocks on the refresh returning,
// so Unmap is strictly after it.
func TestPortMapperCloseJoinsAnInFlightRefresh(t *testing.T) {
	f := newFake()
	f.gate = make(chan struct{})
	f.inflight = make(chan struct{}, 1)
	pm := newPortMapper(f, portmap.UDP, 40404)
	pm.refreshFloor = time.Millisecond
	pm.obtain(context.Background())
	pm.startRefresh()

	<-f.inflight // a refresh is now blocked mid-call inside Refresh

	done := make(chan struct{})
	go func() { pm.close(); close(done) }()

	// close() must be BLOCKED on the join while the refresh is in-flight; give it a moment and
	// confirm it has not completed (no Unmap yet).
	time.Sleep(50 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("close() returned while a refresh was still in-flight — it did not join the goroutine (grill C3/#2)")
	default:
	}
	if atomic.LoadInt32(&f.unmaps) != 0 {
		t.Fatal("Unmap ran before the in-flight refresh finished — the delete raced the refresh")
	}

	close(f.gate) // release the refresh; it returns, the goroutine exits, close() joins and deletes
	<-done
	if atomic.LoadInt32(&f.unmaps) != 1 {
		t.Errorf("Unmap called %d times, want 1", f.unmaps)
	}
	f.mu.Lock()
	if f.refreshAfterUnmap {
		t.Error("a refresh landed after the delete — the join failed")
	}
	f.mu.Unlock()
}

// The integration the mapper exists for (diff-grill #3): a real ceremonyID.close() with a
// portMap set deletes the mapping, exercising setPortMap/close under the mutex the C5 fix added.
// Run under -race (the whole package is) this is the only test that drives that seam.
func TestCeremonyCloseDeletesTheMapping(t *testing.T) {
	f := newFake()
	pm := newPortMapper(f, portmap.UDP, 40404)
	pm.obtain(context.Background())

	cer := &ceremonyID{} // rz and end nil: close() then does mapper.close() and nothing else
	cer.setPortMap(pm)
	cer.close()

	if atomic.LoadInt32(&f.unmaps) != 1 {
		t.Errorf("ceremonyID.close() did not delete the mapping (Unmap called %d times, want 1)", f.unmaps)
	}
}

// The #4 window: close() before setPortMap must still delete a mapping stored afterward.
func TestSetPortMapAfterCloseDeletesImmediately(t *testing.T) {
	f := newFake()
	pm := newPortMapper(f, portmap.UDP, 40404)
	pm.obtain(context.Background())

	cer := &ceremonyID{}
	cer.close()        // teardown races ahead of the arm goroutine's setPortMap
	cer.setPortMap(pm) // must close the mapper immediately, not store it for a close that already ran
	if atomic.LoadInt32(&f.unmaps) != 1 {
		t.Errorf("a mapping stored after close() was not deleted (Unmap %d, want 1) — it would leak to lease expiry (grill #4)", f.unmaps)
	}
}

// TestTheRefreshCadenceNeverOutlivesTheLease — /pending 253, and it closes the item in the
// opposite direction from the one it asked for.
//
// The item asked for a CEILING on the granted lease, to make D15's crash guarantee independent
// of the router. That is not buildable — see refreshAfter's doc — and the last row here is a
// standing guard against someone adding it later. What WAS live on that line is the floor: an
// unconditional 15 s floor applied to a lease shorter than itself schedules the refresh after
// the mapping has already gone, and the published record names a dead port until the next cycle.
func TestTheRefreshCadenceNeverOutlivesTheLease(t *testing.T) {
	const floor = 15 * time.Second

	for _, lease := range []uint32{2, 5, 10, 14} {
		got := refreshAfter(lease, floor)
		want := time.Duration(lease) * time.Second
		if got >= want {
			t.Errorf("granted %ds, refresh scheduled at %v — the mapping expires %v before we "+
				"renew it, and the record we published names a dead port until the next cycle",
				lease, got, got-want)
		}
	}
	// The ordinary case is unchanged: half the grant.
	if got := refreshAfter(120, floor); got != 60*time.Second {
		t.Errorf("refreshAfter(120s) = %v, want 60s — half the granted lease", got)
	}
	// The floor still binds where it is meant to: half a 20 s lease is 10 s, under the floor,
	// and the 15 s floor is still comfortably inside the lease.
	if got := refreshAfter(20, floor); got != floor {
		t.Errorf("refreshAfter(20s) = %v, want the %v floor — half is 10s, under the floor, and "+
			"the floor is still inside the lease", got, floor)
	}
	// No grant reported (the UPnP path, LifetimeObserved false): the floor is all there is.
	if got := refreshAfter(0, floor); got != floor {
		t.Errorf("refreshAfter(no grant) = %v, want the %v floor", got, floor)
	}
	// THE OVERTURN, encoded so it cannot be quietly undone: a long grant is NOT clamped. This
	// row fails the day someone adds /pending 253's proposed ceiling, which would spend gateway
	// round trips renewing a lease with hours left and still not bind what the router holds.
	if got := refreshAfter(7200, floor); got != 3600*time.Second {
		t.Errorf("refreshAfter(7200s) = %v, want 3600s — a cadence clamp cannot bind the lease "+
			"the ROUTER holds, and clamping only buys pointless round trips", got)
	}
}

// TestAGrantedLeaseIsDistinguishedFromARequestedOne — /pending 253's other half.
//
// The UPnP branch recorded `LifetimeSec: DefaultLeaseSec` — our REQUEST — as though the router
// had answered with it, and IGD's AddPortMapping has no lease out-argument to answer with. On
// the mechanism D15 says most consumer routers actually run, Nib was recording a fact about
// itself as a fact about the router.
func TestAGrantedLeaseIsDistinguishedFromARequestedOne(t *testing.T) {
	// NAT-PMP: the reply carries the lifetime, so it IS observed — and a router granting more
	// than we asked for is the case that started this (measured at 7200 against a 120 request).
	resp := make([]byte, 16)
	resp[0], resp[1] = 0, 129 // version 0, response opcode for UDP map
	binary.BigEndian.PutUint16(resp[8:10], 41234)
	binary.BigEndian.PutUint16(resp[10:12], 51234)
	binary.BigEndian.PutUint32(resp[12:16], 7200)
	m, err := portmap.DecodeNATPMPMap(resp, portmap.UDP, 41234)
	if err != nil {
		t.Fatalf("setup: the NAT-PMP reply did not decode: %v", err)
	}
	if !m.LifetimeObserved {
		t.Errorf("a NAT-PMP MAP reply carries the granted lifetime, so it must report as observed")
	}
	if m.LifetimeSec != 7200 {
		t.Fatalf("setup: lifetime = %d, want 7200 — this row is about a router granting MORE "+
			"than the 120 s we request", m.LifetimeSec)
	}
	// And the control that makes the distinction mean something: refreshAfter treats the two
	// identically, because the cadence is not where the difference bites — the crash floor is.
	if refreshAfter(m.LifetimeSec, 15*time.Second) != 3600*time.Second {
		t.Errorf("a 7200 s grant should still be refreshed at half of it")
	}
}

// TestAMapThatFailedAfterSendingIsStillDeleted — /pending 257, at the seam it was filed against.
//
// `obtain` returned on error BEFORE recording anything, and `close()` sent a delete only when
// `have` was true. So a Map whose request reached the router and then failed — a lost reply, a
// cancel, or the two paths that hold a CONFIRMED mapping and drop it — left nothing to delete.
// P05.S07 T02's grill-P1 asked for exactly this and S07's close never ledgered that it was unbuilt.
func TestAMapThatFailedAfterSendingIsStillDeleted(t *testing.T) {
	f := &fakeMapClient{extIP: netip.MustParseAddr("203.0.113.7"), extPort: 51234, failAfterSending: true}
	pm := newPortMapper(f, portmap.UDP, 40404)

	if _, ok := pm.obtain(context.Background()); ok {
		t.Fatal("setup: obtain must fail here — this row is about the error path")
	}
	// SETUP: the request has to have LEFT, or there is nothing to delete and the assertion below
	// would pass on a client that never spoke to the router at all.
	if atomic.LoadInt32(&f.maps) != 1 {
		t.Fatalf("setup: Map was called %d times, want 1", atomic.LoadInt32(&f.maps))
	}
	pm.close()

	if got := atomic.LoadInt32(&f.unmaps); got != 1 {
		t.Errorf("close() sent %d deletes after a Map that failed AFTER its request went out, want 1 — "+
			"the mapping the router may hold lives to lease expiry with nothing able to remove it", got)
	}
}

// TestAConfirmedMappingIsNotDeletedTwice — the other arm of the same rule.
//
// The send-time handle and the confirmed mapping describe ONE mapping, and they are not equal as
// structs: the confirmed one carries the granted external port and lease. Comparing whole structs
// would send two deletes for one mapping, which is why SameTarget exists. Without this row the
// fix above is free to over-delete and nothing would say so.
func TestAConfirmedMappingIsNotDeletedTwice(t *testing.T) {
	f := &fakeMapClient{extIP: netip.MustParseAddr("203.0.113.7"), extPort: 51234}
	pm := newPortMapper(f, portmap.UDP, 40404)

	if _, ok := pm.obtain(context.Background()); !ok {
		t.Fatal("setup: obtain should have succeeded")
	}
	// SETUP: the recorder must have fired, or "not twice" is true for the wrong reason.
	pm.mu.Lock()
	pending := len(pm.pending)
	pm.mu.Unlock()
	if pending != 1 {
		t.Fatalf("setup: %d pending handles recorded, want 1 — the send-time recorder did not fire", pending)
	}
	pm.close()

	if got := atomic.LoadInt32(&f.unmaps); got != 1 {
		t.Errorf("close() sent %d deletes for ONE mapping, want 1", got)
	}
}

// TestAnUnobservedLeaseIsResolvedAndReported — /pending 260, at the seam.
//
// The read-back only means something if the refresh loop actually runs it and something READS
// the result. `LifetimeObserved` had three writers and zero production readers before this — the
// same "published and never consumed" shape this repo deletes fields for — and a value resolved
// into a struct nobody prints does not discharge /pending 258's gate either, which reads "a
// permanent or over-hour lease actually OBSERVED".
func TestAnUnobservedLeaseIsResolvedAndReported(t *testing.T) {
	f := newFake()
	// What the router turns out to hold: a mapping that never expires, against the 120 s asked for.
	f.observed = &portmap.Mapping{
		Protocol: portmap.UDP, InternalPort: 40404, ExternalPort: f.extPort,
		LifetimeSec: portmap.DefaultLeaseSec, LifetimeObserved: true, LeasePermanent: true,
	}
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	pm := newPortMapper(f, portmap.UDP, 40404)
	pm.refreshFloor = 20 * time.Millisecond
	if _, ok := pm.obtain(context.Background()); !ok {
		t.Fatal("setup: obtain failed")
	}
	// SETUP: the mapping must START unobserved, or "it got observed" is true for the wrong reason.
	pm.mu.Lock()
	pre := pm.current.LifetimeObserved
	pm.mu.Unlock()
	if pre {
		t.Fatal("setup: the fake's obtain already reports an observed lease")
	}

	pm.startRefresh()
	// Wait for the STORE, not for the call. Waiting on the call count is a state check on the
	// wrong variable: ObserveLease increments it on entry, and close() racing in before the
	// result is stored leaves `current` unobserved with the counter already at 1 — which is
	// exactly how this row flaked two runs in three.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pm.mu.Lock()
		got := pm.current.LifetimeObserved
		pm.mu.Unlock()
		if got {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	pm.close()

	if got := atomic.LoadInt32(&f.observes); got < 1 {
		t.Fatalf("ObserveLease was called %d times — the refresh loop never resolves a lease the "+
			"obtain could not report, so LifetimeObserved is decoration", got)
	}
	pm.mu.Lock()
	cur := pm.current
	pm.mu.Unlock()
	if !cur.LifetimeObserved || !cur.LeasePermanent {
		t.Errorf("after observation the mapper holds observed=%v permanent=%v — the resolved lease "+
			"was thrown away", cur.LifetimeObserved, cur.LeasePermanent)
	}
	if !strings.Contains(logged.String(), "PERMANENT") {
		t.Errorf("a permanent lease was observed and nothing said so. /pending 258's gate is "+
			"\"actually OBSERVED\", and an observation nobody can read is not evidence. log=%q",
			logged.String())
	}
}

// TestReplacingTheStoredMapperClosesTheOldOne — found by grilling /pending 262.
//
// The republish loop (/pending 256, v1.117.123) turned a one-shot publish into a repeating one,
// and every cycle builds a fresh mapper. `setPortMap` overwrote the field without closing what it
// replaced, so the previous mapper was orphaned — refresh goroutine still running, router mapping
// still installed, and nothing holding a handle to either. Reachable in one ordinary ceremony
// rather than a corner: the republish period is 240 s inside a 300 s connect deadline.
func TestReplacingTheStoredMapperClosesTheOldOne(t *testing.T) {
	first, second := newFake(), newFake()
	pm1, pm2 := newPortMapper(first, portmap.UDP, 40404), newPortMapper(second, portmap.UDP, 40404)
	pm1.obtain(context.Background())
	pm2.obtain(context.Background())

	cer := &ceremonyID{}
	cer.setPortMap(pm1)
	// SETUP: the first mapper must be live and undeleted at the moment it is replaced, or
	// "it got closed" below would be true for the wrong reason.
	if atomic.LoadInt32(&first.unmaps) != 0 {
		t.Fatal("setup: the first mapping was already deleted before it was replaced")
	}
	cer.setPortMap(pm2)

	if got := atomic.LoadInt32(&first.unmaps); got != 1 {
		t.Errorf("replacing the stored mapper left the old one open (Unmap %d, want 1) — its refresh "+
			"goroutine keeps running and its router mapping outlives the ceremony with nothing able "+
			"to delete it", got)
	}
	// And the replacement is the one close() now owns.
	cer.close()
	if got := atomic.LoadInt32(&second.unmaps); got != 1 {
		t.Errorf("the replacement mapping was not deleted by close() (Unmap %d, want 1)", got)
	}
}

// TestAFailedObtainStillClosesItsMapper — the source guard, and it is a source guard because the
// call site builds its own portmap.Client and cannot be driven with a fake.
//
// Since v1.117.120 the mapper records a delete handle for every request that LEFT this host, and
// `close()` is the only thing that drains them. `appendMappedCandidate`'s failure return dropped
// them — the exact leak /pending 257 was built to close, re-opened at a different door by 257's
// own change. The screened-out return four lines below already knew to close; this one did not.
func TestAFailedObtainStillClosesItsMapper(t *testing.T) {
	src, err := os.ReadFile("ceremonynet.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	start, stop := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "mapper := newPortMapper(") {
			start = i
		}
		if start >= 0 && strings.Contains(l, "c.setPortMap(mapper)") {
			stop = i
			break
		}
	}
	if start < 0 || stop < 0 {
		t.Fatal("setup: could not find the mapper's creation and storage in appendMappedCandidate — " +
			"this guard is scanning nothing")
	}
	returns := 0
	for i := start; i < stop; i++ {
		if !strings.Contains(lines[i], "return addrs") {
			continue
		}
		returns++
		closed := false
		for j := i; j >= start && j > i-10; j-- {
			if strings.Contains(lines[j], "mapper.close()") {
				closed = true
				break
			}
		}
		if !closed {
			t.Errorf("ceremonynet.go:%d returns without closing the mapper. Every request that left "+
				"this host is recorded in it and close() is the only thing that drains them, so this "+
				"return drops the delete handles for a mapping the router may well have created.", i+1)
		}
	}
	// STIMULUS: a scan that matched no returns would report a clean bill. Two exist between the
	// mapper's creation and its storage — the obtain failure and the screened-out address.
	if returns < 2 {
		t.Errorf("found %d early return(s) between the mapper's creation and its storage; there are 2, "+
			"so this guard is not reading what it thinks it is", returns)
	}
}

// TestAnObservationSurvivesARefresh — the defect the flaky row above was really reporting.
//
// UPnP's Refresh re-runs the obtain, which reports the lease we ASKED for and marks it
// unobserved. Storing that as-is threw away what ObserveLease had just learned: the mapping
// flipped back to unobserved on every cycle, the loop re-read the lease every tick forever, and
// a permanent lease would have been logged once per tick rather than once. The one-shot
// observation this was supposed to be driven-by-predicate to avoid, arrived by another route.
func TestAnObservationSurvivesARefresh(t *testing.T) {
	f := newFake()
	// A SHORT observed lease on purpose: it keeps the refresh cadence fast, so an observation
	// that gets erased shows up as the loop re-reading it every tick. A long one would stretch
	// the next tick to an hour — correct behaviour, and the first draft's mistake, because with
	// only one refresh there was nothing for the erasure to show up in.
	f.observed = &portmap.Mapping{
		Protocol: portmap.UDP, InternalPort: 40404, ExternalPort: f.extPort,
		LifetimeSec: 1, LifetimeObserved: true,
	}
	pm := newPortMapper(f, portmap.UDP, 40404)
	pm.refreshFloor = 20 * time.Millisecond
	if _, ok := pm.obtain(context.Background()); !ok {
		t.Fatal("setup: obtain failed")
	}
	pm.startRefresh()
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&f.refreshes) < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	pm.close()

	// SETUP: a refresh has to have landed, or "it survived" is trivially true — the erasure
	// happened at the store that follows a refresh. One is enough and one is all there is:
	// carrying a 7200 s lease forward correctly stretches the next tick to an hour, which is
	// the feature working and was itself the first draft's wrong premise.
	if got := atomic.LoadInt32(&f.refreshes); got < 3 {
		t.Fatalf("setup: only %d refresh(es) ran, so an erasure would have had little to show up in", got)
	}
	pm.mu.Lock()
	cur := pm.current
	pm.mu.Unlock()
	if !cur.LifetimeObserved || cur.LifetimeSec != 1 {
		t.Errorf("after %d refresh(es) the mapper holds observed=%v lifetime=%d, want the observed lease "+
			"carried forward — a refresh reports the lease we ASKED for, so storing it as-is erases "+
			"what the read-back learned and the loop re-reads it every tick forever",
			atomic.LoadInt32(&f.refreshes), cur.LifetimeObserved, cur.LifetimeSec)
	}
	// And the observation is not re-fetched once it is held.
	if got := atomic.LoadInt32(&f.observes); got != 1 {
		t.Errorf("ObserveLease ran %d times across %d refresh(es), want exactly 1 — an erased "+
			"observation is re-read every tick, and a permanent lease would be logged every tick with it",
			got, atomic.LoadInt32(&f.refreshes))
	}
}
