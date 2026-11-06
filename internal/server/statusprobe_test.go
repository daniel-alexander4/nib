package server

import (
	"os"
	"strings"
	"testing"
)

// /pending 942: the status route answers a page load, so it must not call the blocking forms of the browser question.
// A source scan, because the failure is a call, and the behaviour is held by TestTheStatusAnswerNeverStartsOrWaitsForTheBrowser.
func TestTheStatusRouteDoesNotCallTheBlockingBrowserQuestion(t *testing.T) {
	b, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, bad := range []string{"pdfops.BrowserAvailable()", "pdfops.FormatAvailable("} {
		if strings.Contains(src, bad) {
			t.Errorf("auth.go calls %s: on a cold cache that starts a real browser inside the page-load route", bad)
		}
	}
	if !strings.Contains(src, "pdfops.BrowserAvailableNow()") || !strings.Contains(src, "pdfops.StatusFormatAvailable(") {
		t.Error("auth.go no longer reads the non-blocking answers")
	}
}
