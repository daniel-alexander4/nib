package server

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// openWindow opens a window stream against ts and returns a function that closes
// it and waits for the server to have noticed.
//
// **The cancel is the point.** `httptest.Server.Close` waits for outstanding
// requests to finish, and this handler finishes only when its client goes away —
// so a test that leaves a stream open does not fail, it HANGS the whole package
// until the test binary's timeout. startServerWith registers that Close as a
// t.Cleanup, and Go runs a test's deferred calls BEFORE its cleanups — which is
// what makes `defer closeIt()` sufficient, and why every caller does it.
func openWindow(t *testing.T, base string) (closeIt func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/window", nil)
	if err != nil {
		cancel()
		t.Fatalf("new request: %v", err)
	}
	// Same-origin is what requirePublicLoopback checks; a browser sends this on
	// every request and Go's client sends none, so the test supplies it.
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("open window stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		resp.Body.Close()
		t.Fatalf("window stream: status %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		cancel()
		resp.Body.Close()
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	return func() {
		cancel()
		resp.Body.Close()
	}
}

// waitForWindows waits until the server's live count reaches want, and fails
// with what it actually saw. Polled rather than slept: the decrement happens in
// the handler's defer after the client cancels, so the two are not ordered.
func waitForWindows(t *testing.T, s *Server, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if got := s.windows.Live(); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("live windows = %d, want %d", s.windows.Live(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestAWindowIsCountedWhileItsStreamIsOpen is the whole of D7 at tier 1: the
// count follows the streams, one for one, in both directions.
func TestAWindowIsCountedWhileItsStreamIsOpen(t *testing.T) {
	ts, s := startServerWith(t)

	if got := s.windows.Live(); got != 0 {
		t.Fatalf("live windows before any connection = %d, want 0", got)
	}

	closeFirst := openWindow(t, ts.URL)
	defer closeFirst()
	waitForWindows(t, s, 1)

	// Two windows count two — the case a boolean cannot represent, and the
	// reason D7 makes this a count. Without it, "one of two closed" and "the
	// last closed" are the same observation.
	closeSecond := openWindow(t, ts.URL)
	waitForWindows(t, s, 2)

	// Closing ONE leaves one. This is the assertion that fails if the handler
	// ever decrements to zero on any disconnect rather than per-stream.
	closeSecond()
	waitForWindows(t, s, 1)

	closeFirst()
	waitForWindows(t, s, 0)
}

// TestTheWindowStreamIsReachableBeforeTheVaultUnlocks pins D3. The server from
// startServerWith has no vault adopted, which is the state a user is in while they
// are looking at the unlock screen — and that user's window must still count,
// or P01.S04 will exit nib while they are typing their passphrase.
func TestTheWindowStreamIsReachableBeforeTheVaultUnlocks(t *testing.T) {
	ts, s := startServerWith(t)

	// The precondition is asserted, not assumed: if this server were already
	// unlocked the test would pass while proving nothing about the locked case.
	s.mu.Lock()
	locked := s.vault == nil
	s.mu.Unlock()
	if !locked {
		t.Fatal("precondition: the test server is already unlocked, so this proves nothing")
	}

	closeIt := openWindow(t, ts.URL)
	defer closeIt()
	waitForWindows(t, s, 1)
}

// TestTheWindowStreamRefusesAForeignOrigin keeps the stream inside the same
// guard as every other public route. It is a GET with no CSRF token, so the
// origin check is the only thing standing between it and any page the user has
// open — and an unbounded stream is a nastier thing to leave reachable than a
// status read.
func TestTheWindowStreamRefusesAForeignOrigin(t *testing.T) {
	ts, _ := startServerWith(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/window", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site window stream: status %d, want 403", resp.StatusCode)
	}
}
