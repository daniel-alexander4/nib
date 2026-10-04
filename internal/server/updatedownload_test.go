package server

import (
	"encoding/json"
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
	ts := serveTest(t, srv)
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
		if !ev.active() && ev.Status != "" {
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
	stubReleaseWith(t, version, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body)
	})
}

// stubReleaseWith is stubRelease with the asset server's behaviour supplied by the test.
func stubReleaseWith(t *testing.T, version string, serveAsset http.HandlerFunc) {
	t.Helper()
	assets := httptest.NewServer(serveAsset)
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
	authedClient(t, ts) // unlocked, so the refusal is the token and not the lock
	for _, path := range []string{"/api/update/download", "/api/update/download/cancel", "/api/update/reveal"} {
		// No CSRF token (a bare client — `c` would attach its own): refused.
		resp, err := (&http.Client{}).Post(ts.URL+path, "application/x-www-form-urlencoded", strings.NewReader("dir=/tmp"))
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

// silentAsset is an asset host that accepts the request and never sends a header until the test ends
// or the client goes away — the shape /pending 589 named.
//
// **It gives up by itself after 15 s**, past every assertion's 10 s deadline. Without that a FAILING
// run deadlocks rather than reporting: `assets.Close` (registered later, so run earlier) waits for
// this handler, and the handler waits on a client the defect never lets go.
func silentAsset(t *testing.T) http.HandlerFunc {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(15 * time.Second):
		}
	}
}

// postDownloadAsync starts the download POST on its own goroutine, because the case under test is
// one where it may not return.
func postDownloadAsync(t *testing.T, ts *httptest.Server, c *http.Client, csrf, dir string) <-chan int {
	t.Helper()
	got := make(chan int, 1)
	go func() {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/update/download",
			strings.NewReader(url.Values{"dir": {dir}}.Encode()))
		if err != nil {
			got <- -1
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", csrf)
		resp, err := c.Do(req)
		if err != nil {
			got <- -1
			return
		}
		resp.Body.Close()
		got <- resp.StatusCode
	}()
	return got
}

// TestASilentReleaseHostIsGivenUpOn — /pending 589/805/821. The header wait had no deadline: a host
// that accepted the connection and never answered held the handler, and the dialog said "Starting…"
// for as long as the kernel kept the socket.
func TestASilentReleaseHostIsGivenUpOn(t *testing.T) {
	old := downloadHeaderTimeout
	downloadHeaderTimeout = 300 * time.Millisecond
	t.Cleanup(func() { downloadHeaderTimeout = old })
	ts, srv, c, csrf := downloadServer(t)
	stubReleaseWith(t, "99.0.0", silentAsset(t))

	got := postDownloadAsync(t, ts, c, csrf, t.TempDir())
	select {
	case code := <-got:
		if code != http.StatusGatewayTimeout {
			t.Errorf("a silent release host answered %d, want 504", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the download handler is still waiting on a host that never answers — nothing bounds the header phase")
	}
	waitForDownload(t, srv, "failed")
}

// TestCancelReachesTheHeaderPhase — /pending 821. `cancel` was registered only after the headers, so
// a Cancel while the host had not answered reported "nothing running" and stopped nothing.
func TestCancelReachesTheHeaderPhase(t *testing.T) {
	ts, srv, c, csrf := downloadServer(t)
	stubReleaseWith(t, "99.0.0", silentAsset(t))

	got := postDownloadAsync(t, ts, c, csrf, t.TempDir())
	deadline := time.Now().Add(10 * time.Second)
	for !srv.dl.snapshot().active() {
		if time.Now().After(deadline) {
			t.Fatal("the download never claimed the slot while waiting for headers, so Cancel has nothing to reach")
		}
		time.Sleep(10 * time.Millisecond)
	}
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/update/download/cancel", "application/json", strings.NewReader("{}"))
	var out struct{ Cancelled bool }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if !out.Cancelled {
		t.Error("Cancel during the header wait reported nothing to cancel")
	}
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("Cancel did not release the handler waiting on the host")
	}
	waitForDownload(t, srv, "cancelled")
}

// TestADownloadDoesNotReplaceAFileThatAppearedWhileItRan — /pending 821. The 412 is a stat before
// the transfer; the door used to rename over whatever appeared at the name meanwhile.
func TestADownloadDoesNotReplaceAFileThatAppearedWhileItRan(t *testing.T) {
	ts, srv, c, csrf := downloadServer(t)
	dir := t.TempDir()
	name := fmt.Sprintf("nib-%s-%s-%s", "99.0.0", runtime.GOOS, runtime.GOARCH)
	planted := []byte("the user's own file")
	stubReleaseWith(t, "99.0.0", func(w http.ResponseWriter, r *http.Request) {
		// Appears after the handler's stat and before the bytes land.
		if err := os.WriteFile(filepath.Join(dir, name), planted, 0o644); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte("release bytes"))
	})
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/update/download",
		"application/x-www-form-urlencoded", strings.NewReader(url.Values{"dir": {dir}}.Encode()))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download answered %d, want 200", resp.StatusCode)
	}
	waitForDownload(t, srv, "failed")
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(planted) {
		t.Fatalf("the download replaced a file that appeared at its name while it ran: now %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".nib-*.tmp")); len(left) != 0 {
		t.Errorf("the refused download left its temp behind: %v", left)
	}
}

// TestALengthlessDownloadStillReportsProgress — /pending 646 #4.
//
// With no `Content-Length` the percent is 0 forever, and the percent was the only throttle key, so
// no progress frame was ever emitted. A whole MiB crossed must wake the windows; a chunk inside one
// must not (the throttle the percent exists for).
func TestALengthlessDownloadStillReportsProgress(t *testing.T) {
	var d downloadState
	if !d.begin(func() {}, "/tmp/x", 0) {
		t.Fatal("setup: the slot was not claimed")
	}
	woke := func(done int64) bool {
		ch := d.changes()
		d.progress(done)
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
	if woke(4096) {
		t.Error("a chunk inside the first MiB woke the windows — the throttle is gone")
	}
	if !woke(3<<20 + 1) {
		t.Error("crossing into the fourth MiB of a length-less transfer woke nobody — no progress is ever shown")
	}
	if got := d.snapshot().Done; got != 3<<20+1 {
		t.Errorf("Done = %d, want %d", got, 3<<20+1)
	}
}
