package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The release download (ADR-039). Dan, 2026-09-16: *"a popup with download progress and a way to
// open/execute/install the new version. Download location should be clearly displayed."*
//
// ── The half these tests own ─────────────────────────────────────────────────
// What the ROUTE does: what it refuses, where it writes, what it leaves behind when it fails, and
// that the URL is the server's own. The dialog — progress rendering, the destination line, Cancel
// owning the abort — is `test/jsdom/downloaddialog.test.mjs`, which has a DOM.
//
// ── Why `assetName` gets its own test ────────────────────────────────────────
// The file name is the one input here that comes from OUTSIDE: it is derived from the asset URL in
// GitHub's JSON. `containedJoin` is the second guard; this is the first, and a test that only
// exercised the happy name would leave the interesting half unproven.

func TestAssetNameRefusesAnythingButAPlainName(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want string
		ok   bool
	}{
		{"https://example.test/d/nib-1.2.3-linux-amd64", "nib-1.2.3-linux-amd64", true},
		{"https://example.test/d/nib_1.2.3_amd64.deb", "nib_1.2.3_amd64.deb", true},
		{"https://example.test/d/nib?x=1", "nib", true},
		{"https://example.test/d/nib#frag", "nib", true},
		// The ones that matter: a name that is not a plain name at all.
		{"https://example.test/d/", "", false},
		{"https://example.test/d/.", "", false},
		{"https://example.test/d/..", "", false},
		{"https://example.test/d/%2e%2e", "", false}, // not decoded here, but must not pass as a name
	} {
		got, ok := assetName(tc.url)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("assetName(%q) = (%q, %v), want (%q, %v)", tc.url, got, ok, tc.want, tc.ok)
		}
	}
	// The property the table is really about, stated so a later edit cannot satisfy the cases
	// above while losing it: nothing that survives may contain a separator.
	for _, u := range []string{"https://x.test/a/b/../../etc/passwd", "https://x.test/a/b%2Fc"} {
		if got, ok := assetName(u); ok && strings.ContainsAny(got, `/\`) {
			t.Errorf("assetName(%q) returned %q, which carries a path separator", u, got)
		}
	}
}

// downloadServer starts a server this file can both DRIVE over HTTP and READ the download state
// from. `startServer` hands back only the httptest wrapper, and the download's outcome lives on the
// Server — it is pushed to windows on a stream rather than published by a route, so a test that
// could only make requests would have to parse SSE to learn whether the transfer finished.
func downloadServer(t *testing.T) (*httptest.Server, *Server, *http.Client, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	srv := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	c, csrf := authedClient(t, ts)
	return ts, srv, c, csrf
}

// waitForDownload blocks until the download reaches a terminal state, and fails if it never does.
//
// **The timeout IS an assertion here**, not harness impatience: ADR-039 requires every exit to
// leave a terminal state, precisely so the dialog cannot spin forever. A run that times out has
// found that defect.
func waitForDownload(t *testing.T, srv *Server, want string) downloadEvent {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ev := srv.dl.snapshot()
		if !ev.Active && ev.Status != "" {
			if ev.Status != want {
				t.Fatalf("the download ended %q (%s), want %q", ev.Status, ev.Problem, want)
			}
			return ev
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the download never reached a terminal state — the dialog would spin forever; last seen %+v",
		srv.dl.snapshot())
	return downloadEvent{}
}

// stubRelease points the update check at a fake GitHub serving one asset for this GOOS/GOARCH, and
// serves the asset itself.
func stubRelease(t *testing.T, version string, body []byte) {
	t.Helper()
	assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body)
	}))
	t.Cleanup(assets.Close)
	// The asset name must match what assetURL looks for on this machine, or the handler answers
	// "no download matches this system" and every assertion below is about the wrong refusal.
	name := fmt.Sprintf("nib-%s-%s-%s", version, runtime.GOOS, runtime.GOARCH)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v%s","html_url":"https://example.test/r","assets":[{"name":%q,"browser_download_url":%q}]}`,
			version, name, assets.URL+"/"+name)
	}))
	t.Cleanup(api.Close)
	old := githubLatestURL
	githubLatestURL = api.URL
	t.Cleanup(func() { githubLatestURL = old })
}

func TestDownloadWritesTheAssetAndReportsWhereItWent(t *testing.T) {
	ts, srv, c, csrf := downloadServer(t)
	body := []byte(strings.Repeat("nib", 4096))
	stubRelease(t, "99.0.0", body) // newer than the test server's "test" version

	dir := t.TempDir()
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/update/download",
		"application/x-www-form-urlencoded", strings.NewReader(url.Values{"dir": {dir}}.Encode()))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download answered %d, want 200", resp.StatusCode)
	}

	// The transfer runs on its own goroutine — the point of returning early — so the assertion is
	// on the terminal state, not on the response.
	ev := waitForDownload(t, srv, "done")
	if ev.Path == "" || filepath.Dir(ev.Path) != dir {
		t.Fatalf("the finished download reports path %q, want a file inside %q — this is the string the "+
			"dialog shows the user as the destination", ev.Path, dir)
	}
	got, err := os.ReadFile(ev.Path)
	if err != nil {
		t.Fatalf("the reported path does not exist: %v", err)
	}
	if len(got) != len(body) {
		t.Errorf("wrote %d bytes, want %d", len(got), len(body))
	}
	// Written NOT executable, and this is the decision rather than an accident: nothing verifies
	// these bytes, so Nib does not hand back something ready to run (ADR-039).
	info, err := os.Stat(ev.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 != 0 {
		t.Errorf("the downloaded file is mode %v — Nib wrote an executable artifact it cannot verify", info.Mode().Perm())
	}
	// And nothing is left in flight.
	if leftovers, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(leftovers) != 0 {
		t.Errorf("a .part file survived a successful download: %v", leftovers)
	}
}

func TestDownloadRefusesWhatItShould(t *testing.T) {
	ts, srv, c, csrf := downloadServer(t)
	stubRelease(t, "99.0.0", []byte("x"))
	dir := t.TempDir()

	post := func(v url.Values) int {
		t.Helper()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/update/download",
			"application/x-www-form-urlencoded", strings.NewReader(v.Encode()))
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// A relative destination is refused by the shared door every folder-writing route uses.
	if code := post(url.Values{"dir": {"relative/path"}}); code != http.StatusBadRequest {
		t.Errorf("a relative destination answered %d, want 400", code)
	}

	// **The client cannot choose the URL.** Sending one must change nothing: the handler resolves
	// the asset itself. If this ever starts being honoured, the route is an arbitrary-download
	// primitive and this test is the thing that says so.
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("PWNED"))
	}))
	defer evil.Close()
	if code := post(url.Values{"dir": {dir}, "url": {evil.URL + "/evil.sh"}, "downloadUrl": {evil.URL + "/evil.sh"}}); code != http.StatusOK {
		t.Fatalf("download answered %d, want 200", code)
	}
	ev := waitForDownload(t, srv, "done")
	if strings.Contains(filepath.Base(ev.Path), "evil") {
		t.Fatalf("the request's url field chose the file: %q", ev.Path)
	}
	if got, _ := os.ReadFile(ev.Path); strings.Contains(string(got), "PWNED") {
		t.Fatal("the request's url field chose the BYTES — this route fetches whatever it is told")
	}

	// A second download onto the same name is refused before any bytes move (412, the same split
	// handleWriteFile draws), so a collision is not discovered at 94%.
	if code := post(url.Values{"dir": {dir}}); code != http.StatusPreconditionFailed {
		t.Errorf("a name collision answered %d, want 412", code)
	}
}

// TestDownloadIsNotReachableWithoutTheVaultAndToken is the gate ADR-039 draws deliberately away
// from the check route's: /api/update/check is public because it only queries out; these write to
// the user's filesystem.
func TestDownloadIsNotReachableWithoutTheVaultAndToken(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)
	for _, path := range []string{"/api/update/download", "/api/update/download/cancel", "/api/update/reveal"} {
		// No CSRF token: refused.
		resp, err := c.Post(ts.URL+path, "application/x-www-form-urlencoded", strings.NewReader("dir=/tmp"))
		if err != nil {
			t.Fatal(err)
		}
		code := resp.StatusCode
		resp.Body.Close()
		if code != http.StatusForbidden {
			t.Errorf("%s without a CSRF token answered %d, want 403 — it writes to disk and must not "+
				"inherit the check route's public gate", path, code)
		}
	}
}

func TestRevealRefusesWhenThereIsNoFinishedDownload(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/update/reveal", "application/json", strings.NewReader("{}"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("reveal with nothing downloaded answered %d, want 409", resp.StatusCode)
	}
}
