package server

import (
	"bufio"
	"context"
	"net/http"
	"testing"
	"time"
)

// ADR-169: Quit tells every open window before the process goes, so each can close itself. Without the frame a
// window learns of a quit as a dropped stream — which is what a crash looks like too.

// openStream opens one window stream and returns its lines once the server has counted it.
func openStream(t *testing.T, ctx context.Context, url string, s *Server, want int64) <-chan lineOrErr {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/window", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := newClient(t).Do(req)
	if err != nil {
		t.Fatalf("open window stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	lines := streamLines(bufio.NewReader(resp.Body))
	readArmedEvent(t, lines) // the first thing a stream carries: the window is connected and in its loop
	deadline := time.Now().Add(2 * time.Second)
	for s.windows.n.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("setup: %d window(s) counted, want %d", s.windows.n.Load(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return lines
}

func postQuit(t *testing.T, url string) time.Duration {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/api/quit", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", url)
	start := time.Now()
	resp, err := newClient(t).Do(req)
	if err != nil {
		t.Fatalf("POST /api/quit: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /api/quit returned %d, want 204", resp.StatusCode)
	}
	return time.Since(start)
}

func TestQuitTellsEveryOpenWindowBeforeTheProcessIsAskedToExit(t *testing.T) {
	ts, s := startServerWith(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := openStream(t, ctx, ts.URL, s, 1)
	second := openStream(t, ctx, ts.URL, s, 2)

	// The exit is watched from before the request: "told first" is an order.
	exited := make(chan time.Time, 1)
	go func() { <-s.IdleExit(); exited <- time.Now() }()

	postQuit(t, ts.URL)

	for i, lines := range []<-chan lineOrErr{first, second} {
		heard := false
		timeout := time.After(2 * time.Second)
	read:
		for {
			select {
			case l := <-lines:
				if l.s == "event: quit" {
					heard = true
					break read
				}
				if l.err != nil {
					break read
				}
			case <-timeout:
				break read
			}
		}
		if !heard {
			t.Errorf("window %d's stream ended without the quit event — that window cannot tell a quit from a crash, and stays open saying it could not reach Nib", i+1)
		}
	}
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Error("Quit answered and the process was never asked to exit")
	}
}

// A window that has stopped answering must not be able to hold a quit: the wait is bounded.
func TestQuitDoesNotWaitForeverForAWindowThatIsNotListening(t *testing.T) {
	ts, s := startServerWith(t)
	// A window the server counts and no stream will ever answer for.
	s.windows.n.Add(1)
	defer s.windows.n.Add(-1)

	took := postQuit(t, ts.URL)
	if took < quitTellWait/2 {
		t.Errorf("Quit answered in %v with a counted window that was never told — it did not wait for it at all (the bound is %v)", took, quitTellWait)
	}
	if took > quitTellWait+2*time.Second {
		t.Errorf("Quit took %v to answer; the wait for windows is bounded at %v", took, quitTellWait)
	}
	select {
	case <-s.IdleExit():
	case <-time.After(2 * time.Second):
		t.Error("Quit gave up waiting and then never asked the process to exit")
	}
}

// And with no window at all there is nothing to wait for.
func TestQuitWithNoWindowAnswersAtOnce(t *testing.T) {
	ts, s := startServerWith(t)
	if took := postQuit(t, ts.URL); took > quitTellWait/2 {
		t.Errorf("Quit took %v with no window open; there is nobody to tell (the bound is %v)", took, quitTellWait)
	}
	select {
	case <-s.IdleExit():
	case <-time.After(2 * time.Second):
		t.Error("the process was never asked to exit")
	}
}
