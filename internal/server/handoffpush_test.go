package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

// TestAHandoffAnsweredByAWindowIsHeardByThatWindow — the launch is told "a window has it" on the
// count, so a window is owed every hand-off from the moment it is counted (/pending 856). The stream
// took its starting point after the count moved, and a hand-off landing in between was answered as
// taken by a window that then never heard it.
func TestAHandoffAnsweredByAWindowIsHeardByThatWindow(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	c, _ := authedClient(t, ts)
	// The launch lands exactly there: the window is counted and its stream has read nothing yet.
	answered := make(chan handoffResponse, 1)
	s.idle.mu.Lock()
	s.idle.countedHook = func() {
		_, out := handoffTo(t, ts, c, "secret", "/nonexistent/x.pdf")
		answered <- out
	}
	s.idle.mu.Unlock()
	lines := listenAsAWindow(t, ts)
	var out handoffResponse
	select {
	case out = <-answered:
	case <-time.After(streamWait(t)):
		t.Fatal("setup: the window connected and the launch behind it never ran")
	}
	if out.Result != "refused" || !out.Surfaced {
		t.Fatalf("setup: the launch = %q surfaced=%v, want refused/true — the window was counted when it asked", out.Result, out.Surfaced)
	}
	if ev, ok := nextHandoff(t, lines, streamWait(t)); !ok || ev.Result != "refused" {
		t.Errorf("the launch was told a window has its refusal, and that window heard %+v (arrived %v) — nothing says it anywhere", ev, ok)
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

// writeImportable puts a PNG on disk: a file Nib converts rather than opens.
func writeImportable(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, testPNG(t, 8, 8), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestAConversionHandedOffBeforeAnyWindowConnectsIsToldToTheFirstWindowOnce (/pending 940 #1): a
// document's own answer carries `importedFrom` only on the route that imported it, so a window that
// connects afterwards and restores the list would never learn the file was converted.
func TestAConversionHandedOffBeforeAnyWindowConnectsIsToldToTheFirstWindowOnce(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	c, _ := authedClient(t, ts)
	if _, out := handoffTo(t, ts, c, "secret", writeImportable(t, "scan.png")); out.Result != "opened" || out.Surfaced {
		t.Fatalf("setup: the launch = %q surfaced=%v, want opened/false (no window yet)", out.Result, out.Surfaced)
	}
	first := listenAsAWindow(t, ts)
	waitForWindows(t, s, 1)
	ev, ok := nextHandoff(t, first, streamWait(t))
	if !ok || ev.Result != "opened" || ev.ImportedFrom != "scan.png" || ev.Seq == 0 {
		t.Fatalf("the first window heard %+v (arrived %v), want the conversion with its sequence", ev, ok)
	}
	second := listenAsAWindow(t, ts)
	waitForWindows(t, s, 2)
	if ev, ok := nextHandoff(t, second, 300*time.Millisecond); ok {
		t.Errorf("a second window was told of the same conversion: %+v", ev)
	}
}

// TestAConversionQueuedWhileLockedIsToldToTheWindowAtUnlock (/pending 940 #2): the drain opens the file
// after the unlock and goes through the same door a live hand-off does.
func TestAConversionQueuedWhileLockedIsToldToTheWindowAtUnlock(t *testing.T) {
	ts, s := startServerWith(t)
	lines := listenAsAWindow(t, ts)
	waitForWindows(t, s, 1)
	if !s.queuePendingOpen(writeImportable(t, "queued.png")) {
		t.Fatal("setup: a locked instance refused to queue")
	}
	_, v := unlockedServer(t)
	s.adoptVault(v)
	ev, ok := nextHandoff(t, lines, streamWait(t))
	if !ok || ev.Result != "opened" || ev.ImportedFrom != "queued.png" {
		t.Fatalf("the window heard %+v (arrived %v), want the queued file's conversion", ev, ok)
	}
}

// ── /pending 946 (a): the window count, the hold and the departure share one lock ──

func conv(p *handoffPush, n *atomic.Int64, name string) int64 { return p.tell(n, "opened", "", name) }

// A conversion announced with no window open is owed to the first window and comes back APART from the stream's start:
// nothing announced since is replayed (I2), and it is owed once (I1).
func TestAHeldConversionIsOwedOnceAndReplaysNothingElse(t *testing.T) {
	var p handoffPush
	var n atomic.Int64
	conv(&p, &n, "a.docx") // seq 1, held: no window
	n.Add(1)               // a window already open for the next two
	p.tell(&n, "refused", "no", "")
	p.tell(&n, "focused", "", "")
	n.Add(-1)
	from, owed, count := p.arrive(&n)
	if len(owed) != 1 || owed[0].importedFrom != "a.docx" || count != 1 || from != 3 {
		t.Fatalf("arrive: from %d, owed %+v, count %d; want from 3 (the refusal and the focus are NOT replayed), one owed", from, owed, count)
	}
	if got := p.since(from); len(got) != 0 {
		t.Errorf("since(from) = %+v, want nothing", got)
	}
	n.Add(-1)
	if _, again, _ := p.arrive(&n); len(again) != 0 {
		t.Errorf("the next window was owed %+v again", again)
	}
}

// A window counted between the decision and the hold used to leave the note delivered by sequence AND still held, so a
// reload replayed it (I1). Whichever side of the other each lands on, the conversion reaches the window exactly once and
// nothing stays held while a window is open.
func TestAWindowArrivingWhileAConversionIsAnnouncedHearsItExactlyOnce(t *testing.T) {
	for i := 0; i < 3000; i++ {
		var p handoffPush
		var n atomic.Int64
		var from uint64
		var owed []handoffNote
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); conv(&p, &n, "a.docx") }()
		go func() { defer wg.Done(); from, owed, _ = p.arrive(&n) }()
		wg.Wait()
		heard := len(owed) + len(p.since(from))
		if heard != 1 {
			t.Fatalf("run %d: the conversion reached the window %d times (owed %d, since %d)", i, heard, len(owed), len(p.since(from)))
		}
		for _, ev := range p.recent {
			if ev.held {
				t.Fatalf("run %d: a note stays held with a window open: it would be replayed to the next", i)
			}
		}
	}
}

// The last window leaving takes with it a conversion it was never sent (I3): held again, for the next window. One it had
// heard is not.
func TestTheLastWindowLeavingHoldsWhatItNeverHeard(t *testing.T) {
	var p handoffPush
	var n atomic.Int64
	n.Add(1)
	conv(&p, &n, "late.docx") // announced to a window that is about to go
	if left := p.depart(&n, 0); left != 0 {
		t.Fatalf("left = %d", left)
	}
	if !p.recent[0].held {
		t.Error("a conversion the last window never heard was not held for the next")
	}
	n.Store(1)
	p.recent[0].held = false
	p.depart(&n, 1)
	if p.recent[0].held {
		t.Error("control: a conversion the window had heard was held again")
	}
	n.Store(2)
	p.recent[0].held = false
	p.depart(&n, 0)
	if p.recent[0].held {
		t.Error("control: a window leaving while another is open held a note")
	}
}

// The bound drops the oldest note that is not owed, never an owed conversion while a heard refusal could go (the
// eviction half of 946 (a)).
func TestTheBoundKeepsTheConversionsOwedToAWindow(t *testing.T) {
	var p handoffPush
	var none, one atomic.Int64
	one.Store(1)
	conv(&p, &none, "first.docx") // oldest, held
	for i := 0; i < handoffKept; i++ {
		p.tell(&one, "focused", "", "") // heard by the open window
	}
	if len(p.recent) != handoffKept {
		t.Fatalf("%d notes kept, want the bound %d", len(p.recent), handoffKept)
	}
	if !p.recent[0].held || p.recent[0].importedFrom != "first.docx" {
		t.Errorf("the owed conversion was evicted: %+v", p.recent[0])
	}
}
