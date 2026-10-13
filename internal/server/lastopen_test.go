package server

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// graceAfterAWindow drives the real route to the state a closed Nib is in: one window came and
// went, and the grace has been counting down for longer than a window is ever merely away
// (`launchGapBound`, /pending 847). The grace is aged, not waited for.
func graceAfterAWindow(t *testing.T, tsURL string, s *Server) {
	t.Helper()
	youngGraceAfterAWindow(t, tsURL, s)
	ageGrace(s, launchGapBound)
}

// ageGrace moves the running grace's start back by d, under the lock that owns it.
func ageGrace(s *Server, d time.Duration) {
	s.idle.mu.Lock()
	s.idle.armedAt = s.idle.armedAt.Add(-d)
	s.idle.mu.Unlock()
}

// youngGraceAfterAWindow is the state a reload or a stream reconnect passes through: the window
// went a moment ago and the grace has only just been armed.
func youngGraceAfterAWindow(t *testing.T, tsURL string, s *Server) {
	t.Helper()
	closeIt := openWindow(t, tsURL)
	waitForWindows(t, s, 1)
	closeIt()
	waitForWindows(t, s, 0)
	deadline := time.Now().Add(2 * time.Second)
	for !s.idleGraceRunning() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !s.idleGraceRunning() {
		t.Fatal("setup: the window went and no grace is running, so nothing below is a launch after a close")
	}
}

func openPaths(s *Server) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, d := range s.docs {
		out = append(out, d.path)
	}
	return out
}

// TestALaunchAfterTheLastWindowClosedStartsFresh — ADR-085. Close Nib, start it again inside the
// grace: the launch is handed to the old process, and it must not come up holding what the closed
// window had.
func TestALaunchAfterTheLastWindowClosedStartsFresh(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	s.ArmIdleExit(true)
	c, _ := authedClient(t, ts)
	a, b, fresh := pdfOnDisk(t), pdfOnDisk(t), pdfOnDisk(t)
	for _, p := range []string{a, b} {
		if code, out := handoffTo(t, ts, c, "secret", p); code != http.StatusOK || out.Result != "opened" {
			t.Fatalf("setup: opening %s = %d/%q", p, code, out.Result)
		}
	}
	if got := openPaths(s); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("setup: open = %v, want the two just handed off — nothing below would be closing anything", got)
	}

	graceAfterAWindow(t, ts.URL, s)

	if code, out := handoffTo(t, ts, c, "secret", fresh); code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("the launch = %d/%q, want 200/opened", code, out.Result)
	}
	if got := openPaths(s); !slices.Equal(got, []string{fresh}) {
		t.Errorf("after closing Nib and starting it again, open = %v, want only %s — the documents "+
			"of the window that closed came back", got, fresh)
	}
	if _, byHandoff := s.IdleExitCancels(); byHandoff != 1 {
		t.Errorf("hand-off cancels = %d, want 1: the launch must still keep the process alive", byHandoff)
	}
}

// TestALaunchInsideAReconnectGapKeepsTheDocuments — /pending 847. A grace also runs while a window
// is only away (a reload, one stream reconnect), and a launch fits inside that gap: it must keep the
// process alive and close nothing, because the window is still on screen with its edits.
func TestALaunchInsideAReconnectGapKeepsTheDocuments(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	s.ArmIdleExit(true)
	c, _ := authedClient(t, ts)
	a, fresh := pdfOnDisk(t), pdfOnDisk(t)
	if code, out := handoffTo(t, ts, c, "secret", a); code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("setup: %d/%q", code, out.Result)
	}
	// An hour, so the test's own pace can never carry the grace past the bound.
	defer func(d time.Duration) { launchGapBound = d }(launchGapBound)
	launchGapBound = time.Hour

	youngGraceAfterAWindow(t, ts.URL, s)
	if code, out := handoffTo(t, ts, c, "secret", fresh); code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("the launch = %d/%q, want 200/opened", code, out.Result)
	}
	if got := openPaths(s); !slices.Equal(got, []string{a, fresh}) {
		t.Errorf("a launch inside a young grace left open = %v, want %v — a launch that landed in a "+
			"reload or reconnect gap closed the documents of a window still on screen", got, []string{a, fresh})
	}
	if _, byHandoff := s.IdleExitCancels(); byHandoff != 1 {
		t.Errorf("hand-off cancels = %d, want 1: a launch in the gap must still keep the process alive", byHandoff)
	}
	if s.idleGraceRunning() {
		t.Error("the grace is still running after the launch; the process would exit under it")
	}

	// The same grace one bound older is a closed Nib, and the launch ends the session (ADR-085).
	youngGraceAfterAWindow(t, ts.URL, s)
	ageGrace(s, launchGapBound)
	if code, out := handoffTo(t, ts, c, "secret", ""); code != http.StatusOK || out.Result != "window" {
		t.Fatalf("the later launch = %d/%q, want 200/window", code, out.Result)
	}
	if got := openPaths(s); len(got) != 0 {
		t.Errorf("a launch into a grace one bound old left %v open, want nothing — the bound is "+
			"where a closed Nib begins", got)
	}
}

// TestTheLaunchGapBoundExceedsOneReconnect — the gap a window leaves is at least one `sseRetry`, so
// a bound at or under it would not cover the blip it exists for; and it must leave the grace room.
func TestTheLaunchGapBoundExceedsOneReconnect(t *testing.T) {
	if launchGapBound <= sseRetry {
		t.Errorf("launchGapBound = %s, want more than one reconnect (%s): a launch landing in a stream "+
			"blip would still end the session", launchGapBound, sseRetry)
	}
	if launchGapBound >= idleExitGrace {
		t.Errorf("launchGapBound = %s reaches the grace (%s): no launch could ever end a session", launchGapBound, idleExitGrace)
	}
}

// TestALaunchWithNoFileAfterTheLastWindowClosedOpensNothing — the same boundary for a bare start.
func TestALaunchWithNoFileAfterTheLastWindowClosedOpensNothing(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	s.ArmIdleExit(true)
	c, _ := authedClient(t, ts)
	if code, out := handoffTo(t, ts, c, "secret", pdfOnDisk(t)); code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("setup: %d/%q", code, out.Result)
	}
	graceAfterAWindow(t, ts.URL, s)
	if code, out := handoffTo(t, ts, c, "secret", ""); code != http.StatusOK || out.Result != "window" {
		t.Fatalf("a bare launch = %d/%q, want 200/window", code, out.Result)
	}
	if got := openPaths(s); len(got) != 0 {
		t.Errorf("a bare start after closing Nib holds %v, want nothing", got)
	}
}

// TestAReloadKeepsItsDocuments — the other side of the boundary, and the one the fix must not
// move: a window that comes back with NO launch is a reload.
func TestAReloadKeepsItsDocuments(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	s.ArmIdleExit(true)
	c, _ := authedClient(t, ts)
	a := pdfOnDisk(t)
	if code, out := handoffTo(t, ts, c, "secret", a); code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("setup: %d/%q", code, out.Result)
	}
	graceAfterAWindow(t, ts.URL, s)
	back := openWindow(t, ts.URL)
	defer back()
	waitForWindows(t, s, 1)
	if got := openPaths(s); !slices.Equal(got, []string{a}) {
		t.Errorf("after a reload, open = %v, want %s — a refresh closed the user's document", got, a)
	}
}

// TestALaunchWhileAWindowIsOpenClosesNothing — a launch is a new session only when no window is
// left. With one open it adds, as it always has.
func TestALaunchWhileAWindowIsOpenClosesNothing(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	s.ArmIdleExit(true)
	c, _ := authedClient(t, ts)
	a, b := pdfOnDisk(t), pdfOnDisk(t)
	if code, out := handoffTo(t, ts, c, "secret", a); code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("setup: %d/%q", code, out.Result)
	}
	closeIt := openWindow(t, ts.URL)
	defer closeIt()
	waitForWindows(t, s, 1)
	if code, out := handoffTo(t, ts, c, "secret", b); code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("the launch = %d/%q", code, out.Result)
	}
	if got := openPaths(s); !slices.Equal(got, []string{a, b}) {
		t.Errorf("open = %v, want both — a launch closed documents under an open window", got)
	}
}

func lastOpenOver(t *testing.T, c *http.Client, base string) []string {
	t.Helper()
	resp, err := c.Get(base + "/api/lastopen")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/lastopen = %d", resp.StatusCode)
	}
	var entries []recentEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if entries == nil {
		t.Fatal("/api/lastopen answered null; the client treats it as a list")
	}
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

// TestTheLastWindowGoingRecordsWhatWasOpen — and a launch that then closes those documents leaves
// the record, which is the whole point of having one.
func TestTheLastWindowGoingRecordsWhatWasOpen(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	s.ArmIdleExit(true)
	c, _ := authedClient(t, ts)
	if got := lastOpenOver(t, c, ts.URL); len(got) != 0 {
		t.Fatalf("setup: a fresh vault already records %v", got)
	}
	a, b := pdfOnDisk(t), pdfOnDisk(t)
	for _, p := range []string{a, b} {
		if code, out := handoffTo(t, ts, c, "secret", p); code != http.StatusOK || out.Result != "opened" {
			t.Fatalf("setup: %d/%q", code, out.Result)
		}
	}
	upload, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	addDocument(s, upload) // no file behind it, so nothing to reopen

	graceAfterAWindow(t, ts.URL, s)
	if got := lastOpenOver(t, c, ts.URL); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("recorded %v, want the two files in tab order and not the upload", got)
	}

	// The launch closes them; the record stays.
	if code, _ := handoffTo(t, ts, c, "secret", ""); code != http.StatusOK {
		t.Fatalf("the launch = %d", code)
	}
	if got := lastOpenOver(t, c, ts.URL); !slices.Equal(got, []string{a, b}) {
		t.Errorf("after the launch closed them the record is %v, want it unchanged", got)
	}

	// A session that ends with nothing open does not cost the user the one before it.
	graceAfterAWindow(t, ts.URL, s)
	if got := lastOpenOver(t, c, ts.URL); !slices.Equal(got, []string{a, b}) {
		t.Errorf("a window that closed with nothing open replaced the record with %v", got)
	}
}

// TestQuitRecordsBeforeItExits — the exit closes the window stream, and the teardown does not wait
// for that handler's vault write.
func TestQuitRecordsBeforeItExits(t *testing.T) {
	ts, s := startServerWith(t)
	s.SetHandoffSecret("secret")
	c, csrf := authedClient(t, ts)
	a := pdfOnDisk(t)
	if code, out := handoffTo(t, ts, c, "secret", a); code != http.StatusOK || out.Result != "opened" {
		t.Fatalf("setup: %d/%q", code, out.Result)
	}
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/quit", "", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("quit = %d", resp.StatusCode)
	}
	if got := s.unlockedVault().LastOpen(); !slices.Equal(got, []string{a}) {
		t.Errorf("by the time Quit answered, the record was %v, want %s", got, a)
	}
}
