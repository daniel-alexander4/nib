package server

import (
	"net/http"
	"testing"
)

// TestAHandoffToAnExitingInstanceIsRefused — /pending 727. Once the process has decided to exit, a
// hand-off has no grace left to cancel; answering it opened the document in a process about to
// tear down, and the user double-clicked a PDF into a dead tab. It is refused with a non-200, which
// sends the launch down its become-primary path (`handedOff` in cmd/nib) to serve the file itself.
func TestAHandoffToAnExitingInstanceIsRefused(t *testing.T) {
	ts, srv := startServerWith(t)
	c, _ := authedClient(t, ts)
	srv.SetHandoffSecret("secret")
	path := pdfOnDisk(t)

	// The stimulus's control: the same hand-off to a live process opens the document.
	if code, out := handoffTo(t, ts, c, "secret", path); code != http.StatusOK || out.Result == "refused" {
		t.Fatalf("setup: a hand-off to a live instance = %d/%q, want 200 and an open", code, out.Result)
	}
	srv.mu.Lock()
	before := len(srv.docs)
	srv.mu.Unlock()

	srv.RequestExit(exitCauseLastWindow)
	code, _ := handoffTo(t, ts, c, "secret", pdfOnDisk(t))
	if code != http.StatusServiceUnavailable {
		t.Errorf("a hand-off to an instance that has decided to exit = %d, want 503 — answering it "+
			"opens the document in a process that is about to exit", code)
	}
	srv.mu.Lock()
	after := len(srv.docs)
	srv.mu.Unlock()
	if after != before {
		t.Errorf("the exiting instance opened the handed-off document (%d → %d documents)", before, after)
	}
}
