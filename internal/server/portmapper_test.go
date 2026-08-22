package server

import (
	"context"
	"net/netip"
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

	mu                sync.Mutex
	unmapped          bool
	refreshAfterUnmap bool // set if a Refresh is ever seen after an Unmap — the C3 resurrection
}

func (f *fakeMapClient) Map(ctx context.Context, proto portmap.Protocol, internalPort uint16) (portmap.Mapping, netip.Addr, error) {
	atomic.AddInt32(&f.maps, 1)
	// LifetimeSec 1 so wait = 1/2 = 0 is floored to refreshFloor — the test governs the
	// cadence via refreshFloor, not a 1-second real interval that races a 2s deadline.
	return portmap.Mapping{Protocol: proto, InternalPort: internalPort, ExternalPort: f.extPort, LifetimeSec: 1}, f.extIP, nil
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

	cer := &ceremonyID{} // rz and end nil: close() then does stopNet (nil) + mapper.close()
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
