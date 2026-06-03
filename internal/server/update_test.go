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

func TestAssetURL(t *testing.T) {
	assets := []asset{
		{Name: "nib-0.9.25-linux-amd64", URL: "u/lin-amd64"},
		{Name: "nib-0.9.25-linux-arm64", URL: "u/lin-arm64"},
		{Name: "nib-0.9.25-darwin-arm64", URL: "u/dar-arm64"},
		{Name: "nib-0.9.25-windows-amd64.exe", URL: "u/win-amd64"},
		{Name: "nib_0.9.25_amd64.deb", URL: "u/deb-amd64"},
		{Name: "nib_0.9.25_arm64.deb", URL: "u/deb-arm64"},
	}
	cases := []struct {
		goos, goarch string
		managed      bool
		want         string
	}{
		{"linux", "amd64", false, "u/lin-amd64"},
		{"linux", "arm64", false, "u/lin-arm64"},
		{"darwin", "arm64", false, "u/dar-arm64"},
		{"windows", "amd64", false, "u/win-amd64"},
		{"linux", "amd64", true, "u/deb-amd64"}, // managed -> the .deb, not the raw binary
		{"linux", "arm64", true, "u/deb-arm64"},
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
