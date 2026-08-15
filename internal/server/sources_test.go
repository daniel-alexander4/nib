package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// An empty history must serialize as [] rather than null — the client reads it
// as a list (recent.length), and a null body crashed the Recent menu.
func TestRecentEmptyIsArrayNotNull(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)

	resp, err := c.Get(ts.URL + "/api/recent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if got := strings.TrimSpace(string(body)); got != "[]" {
		t.Errorf("empty recent body = %q, want %q", got, "[]")
	}
}

func TestRecentReflectsOpen(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	resp, err := c.Get(ts.URL + "/api/recent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var recent []recentEntry
	if err := json.NewDecoder(resp.Body).Decode(&recent); err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Path != path {
		t.Errorf("recent = %v, want one entry with path %s", recent, path)
	}
	// The display name comes from the server: the browser cannot tell where a
	// Windows path ends, and stripping to the last "/" left C:\a\b.pdf whole.
	if len(recent) == 1 && recent[0].Name != filepath.Base(path) {
		t.Errorf("recent name = %q, want %q", recent[0].Name, filepath.Base(path))
	}
}

// Opening a second file by path must keep the first in the history (newest
// first), not replace it.
func TestRecentKeepsPriorOpens(t *testing.T) {
	ts, pathA := startServer(t)
	c, csrf := authedClient(t, ts)

	data, _ := testpdf.Form()
	pathB := filepath.Join(t.TempDir(), "second.pdf")
	if err := os.WriteFile(pathB, data, 0o600); err != nil {
		t.Fatal(err)
	}

	openByPath(t, ts.URL, c, csrf, pathA)
	openByPath(t, ts.URL, c, csrf, pathB)

	resp, err := c.Get(ts.URL + "/api/recent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var recent []recentEntry
	if err := json.NewDecoder(resp.Body).Decode(&recent); err != nil {
		t.Fatal(err)
	}
	want := []string{pathB, pathA}
	if len(recent) != 2 || recent[0].Path != want[0] || recent[1].Path != want[1] {
		t.Errorf("recent = %v, want paths %v", recent, want)
	}
}

func TestOpenURLRejectsNonPDF(t *testing.T) {
	// A data: URL isn't fetchable here, so we exercise the bad-input path: an
	// empty/garbage URL must yield an error status, never a panic.
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open-url", "application/json",
		jsonBody(map[string]string{"url": "http://127.0.0.1:1/nope.pdf"}))
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Errorf("open-url of unreachable URL returned 200, want an error")
	}
}
