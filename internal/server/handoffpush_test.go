package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// ADR-086 — a launch into a running Nib is a tab in the window that is already open.

// listenAsAWindow opens a window stream and returns its lines; the stream counts as a window.
func listenAsAWindow(t *testing.T, ts *httptest.Server) <-chan lineOrErr {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/window", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := newClient(t).Do(req)
	if err != nil {
		t.Fatalf("open window stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return streamLines(bufio.NewReader(resp.Body))
}

// nextHandoff reads the next `event: handoff` frame, or reports that none came within wait.
func nextHandoff(t *testing.T, lines <-chan lineOrErr, wait time.Duration) (handoffEvent, bool) {
	t.Helper()
	deadline := time.After(wait)
	seen := false
	for {
		select {
		case <-deadline:
			return handoffEvent{}, false
		case l := <-lines:
			if l.err != nil {
				t.Fatalf("read stream: %v", l.err)
			}
			switch {
			case l.s == "event: handoff":
				seen = true
			case seen && strings.HasPrefix(l.s, "data: "):
				var ev handoffEvent
				if err := json.Unmarshal([]byte(strings.TrimPrefix(l.s, "data: ")), &ev); err != nil {
					t.Fatalf("handoff event did not decode: %v (%q)", err, l.s)
				}
				return ev, true
			}
		}
	}
}

// streamWait is how long a pushed event is waited for: "is it pushing at all", never "how fast".
func streamWait(t *testing.T) time.Duration {
	wait := 30 * time.Second
	if d, ok := t.Deadline(); ok {
		if q := time.Until(d) / 4; q < wait {
			wait = q
		}
	}
	return max(wait, 5*time.Second)
}

// TestALaunchWhileAWindowIsOpenIsPushedToThatWindow is the request itself: the open window hears
// the document, and the launch is told not to open another.
func TestALaunchWhileAWindowIsOpenIsPushedToThatWindow(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	c, _ := authedClient(t, ts)
	lines := listenAsAWindow(t, ts)
	waitForWindows(t, s, 1)

	a := pdfOnDisk(t)
	code, out := handoffTo(t, ts, c, "secret", a)
	if code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("the launch = %d/%q", code, out.Result)
	}
	if !out.Surfaced {
		t.Error("a window is open and the launch was not told so — it opens a second window showing every document")
	}
	ev, ok := nextHandoff(t, lines, streamWait(t))
	if !ok || ev.Result != "opened" {
		t.Fatalf("the open window heard %+v (arrived %v), want an opened hand-off — it stays on its old tabs and never shows the document", ev, ok)
	}

	// The same file again is "focused", and two identical outcomes are still two events.
	for i := 0; i < 2; i++ {
		if _, out := handoffTo(t, ts, c, "secret", a); out.Result != "focused" || !out.Surfaced {
			t.Fatalf("relaunch %d = %q surfaced=%v, want focused/true", i, out.Result, out.Surfaced)
		}
		if ev, ok := nextHandoff(t, lines, streamWait(t)); !ok || ev.Result != "focused" {
			t.Fatalf("relaunch %d: the window heard %+v (arrived %v), want focused — a repeated outcome was swallowed", i, ev, ok)
		}
	}

	// A refusal reaches the open window too, as the notice the launch has no terminal to print.
	if _, out := handoffTo(t, ts, c, "secret", "/nonexistent/x.pdf"); out.Result != "refused" || !out.Surfaced {
		t.Fatalf("a refused launch = %q surfaced=%v, want refused/true", out.Result, out.Surfaced)
	}
	if ev, ok := nextHandoff(t, lines, streamWait(t)); !ok || ev.Result != "refused" {
		t.Errorf("the window heard %+v (arrived %v), want the refusal", ev, ok)
	}
}

// TestALaunchWithNoDocumentAlwaysGetsAWindow — it is how a page that never held the session gets
// in, so an open window somewhere else does not answer it.
func TestALaunchWithNoDocumentAlwaysGetsAWindow(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	c, _ := authedClient(t, ts)
	lines := listenAsAWindow(t, ts)
	waitForWindows(t, s, 1)
	_, out := handoffTo(t, ts, c, "secret", "")
	if out.Result != "window" || out.Surfaced || out.Launch == "" {
		t.Errorf("a launch with no document = %q surfaced=%v key=%q, want window/false and a key", out.Result, out.Surfaced, out.Launch)
	}
	if ev, ok := nextHandoff(t, lines, 300*time.Millisecond); ok {
		t.Errorf("the open window was told of a launch that carried nothing: %+v", ev)
	}
}

// TestALaunchWaitsForAWindowAlreadyOnItsWay — several files opened together start one launch each,
// and all but the first arrive before the first one's window has connected.
func TestALaunchWaitsForAWindowAlreadyOnItsWay(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	c, _ := authedClient(t, ts)
	a, b := pdfOnDisk(t), pdfOnDisk(t)

	// Nobody asked for a window and none is open: this launch must open one.
	if _, out := handoffTo(t, ts, c, "secret", a); out.Result != "opened" || out.Surfaced {
		t.Fatalf("with no window anywhere, the launch = %q surfaced=%v, want opened/false — nothing would show the document", out.Result, out.Surfaced)
	}
	// That answer asked for one, so the next launch waits for it.
	if _, out := handoffTo(t, ts, c, "secret", b); out.Result != "opened" || !out.Surfaced {
		t.Errorf("a launch behind one that was told to open a window = %q surfaced=%v, want opened/true — one window per file", out.Result, out.Surfaced)
	}
	// A refusal is a sentence, and a window still on its way was never told it.
	if _, out := handoffTo(t, ts, c, "secret", "/nonexistent/x.pdf"); out.Result != "refused" || out.Surfaced {
		t.Errorf("a refusal with no window open = %q surfaced=%v, want refused/false — the refusal reaches nobody", out.Result, out.Surfaced)
	}

	// The process's own launch counts as asking, and a browser that could not start withdraws it.
	_, s2 := startServerWith(t)
	if s2.push.expected() {
		t.Fatal("setup: a fresh server already expects a window")
	}
	s2.ExpectWindow()
	if !s2.push.expected() {
		t.Error("ExpectWindow did not register")
	}
	s2.ExpectNoWindow()
	if s2.push.expected() {
		t.Error("ExpectNoWindow left the expectation standing — a launch would wait for a window that cannot come")
	}

	// A window that never came stops being waited for.
	s.push.mu.Lock()
	s.push.expectUntil = time.Now().Add(-time.Second)
	s.push.mu.Unlock()
	if _, out := handoffTo(t, ts, c, "secret", a); out.Surfaced {
		t.Error("a window that never arrived is still being waited for; every later launch shows nothing")
	}
}

// TestAWindowHearsOnlyTheHandoffsThatArriveWhileItIsOpen — its boot restore already shows the
// documents from before, and a refusal replayed to a later window is a notice about nothing.
func TestAWindowHearsOnlyTheHandoffsThatArriveWhileItIsOpen(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	c, _ := authedClient(t, ts)
	early := listenAsAWindow(t, ts)
	waitForWindows(t, s, 1)
	handoffTo(t, ts, c, "secret", "/nonexistent/x.pdf")
	if _, ok := nextHandoff(t, early, streamWait(t)); !ok {
		t.Fatal("setup: the window that was open did not hear the hand-off")
	}
	// A window was asked for; the one connecting now is it.
	s.ExpectWindow()
	late := listenAsAWindow(t, ts)
	waitForWindows(t, s, 2)
	if ev, ok := nextHandoff(t, late, 300*time.Millisecond); ok {
		t.Errorf("a window that connected afterwards was replayed %+v", ev)
	}
	// And a connecting window ends the wait for one — or a launch after that window closed would
	// be told a window is on its way, and show nothing.
	if s.push.expected() {
		t.Error("the expected window connected and one is still expected")
	}
}

// TestAQueuedDocumentDoesNotOutliveItsSession — a file handed to a LOCKED Nib whose windows were
// then closed belonged to that session; the next launch opens its own document and no other.
func TestAQueuedDocumentDoesNotOutliveItsSession(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	s.ArmIdleExit(true)
	c := newClient(t)
	a, b := pdfOnDisk(t), pdfOnDisk(t)
	if _, out := handoffTo(t, ts, c, "secret", a); out.Result != "queued" {
		t.Fatalf("setup: a hand-off to a locked Nib = %q, want queued", out.Result)
	}
	graceAfterAWindow(t, ts.URL, s)
	if _, out := handoffTo(t, ts, c, "secret", b); out.Result != "queued" {
		t.Fatalf("the launch after the close = %q, want queued", out.Result)
	}
	s.mu.Lock()
	queued := slices.Clone(s.pendingOpens)
	s.mu.Unlock()
	if !slices.Equal(queued, []string{b}) {
		t.Errorf("queued for the unlock = %v, want only %s — the closed session's document opens beside it", queued, b)
	}
}
