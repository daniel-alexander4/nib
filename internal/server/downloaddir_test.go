package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/vault"
)

// Where an update is downloaded to (ADR-102). Dan, 2026-10-07: *"nib download folder should always
// be the browser default folder unless configured differently in settings."*
//
// ── The half these tests own ─────────────────────────────────────────────────
// The ORDER of the rungs and what makes one give way, and the Settings field's refusals. What a
// browser's own preference file says — and what a hostile one is allowed to say — is
// `internal/browser/downloads_test.go`; both readers of the machine are stubbed here.

func TestDownloadDirTriesEachRungInOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	set, inBrowser, system := t.TempDir(), t.TempDir(), t.TempDir()
	gone := filepath.Join(t.TempDir(), "not-there")
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nib := filepath.Join(home, "nib")

	for _, tc := range []struct {
		name                       string
		setting, browser, system   string
		wantDir, wantFrom, wantWho string
	}{
		{"the setting wins over everything", set, inBrowser, system, set, downloadFromSetting, ""},
		{"no setting: the browser's folder", "", inBrowser, system, inBrowser, downloadFromBrowser, "Chrome"},
		{"browser has none set: the system Downloads folder", "", "", system, system, downloadFromSystem, "Chrome"},
		{"nothing anywhere: Nib's own folder", "", "", "", nib, downloadFromNib, "Chrome"},
		// A folder that is not there is skipped, never created — each rung in turn.
		{"a setting that is gone gives way", gone, inBrowser, system, inBrowser, downloadFromBrowser, "Chrome"},
		{"a setting that is a file gives way", file, inBrowser, system, inBrowser, downloadFromBrowser, "Chrome"},
		{"a browser folder that is gone gives way", "", gone, system, system, downloadFromSystem, "Chrome"},
		{"a relative browser folder gives way", "", "Downloads", system, system, downloadFromSystem, "Chrome"},
		{"a system folder that is gone gives way", "", "", gone, nib, downloadFromNib, "Chrome"},
	} {
		stubDownloadSources(t, tc.browser, "Chrome", tc.system)
		got := downloadDir(vault.Settings{DownloadDir: tc.setting})
		if got.Dir != tc.wantDir || got.From != tc.wantFrom || got.Browser != tc.wantWho {
			t.Errorf("%s: got %+v, want {%s %s %s}", tc.name, got, tc.wantDir, tc.wantFrom, tc.wantWho)
		}
	}
	// Skipped means NOT created: nothing above may have brought the missing folder into being.
	if _, err := os.Stat(gone); err == nil {
		t.Error("resolving the folder created one that was not there")
	}
}

// TestTheDownloadGoesWhereTheBrowserDownloads is the request itself, end to end through the route:
// nothing set in Nib, the browser says a folder, the file lands in it — and the request named none.
func TestTheDownloadGoesWhereTheBrowserDownloads(t *testing.T) {
	ts, srv, c, csrf := downloadServer(t)
	stubRelease(t, "99.0.0", []byte("release"))
	inBrowser := t.TempDir()
	stubDownloadSources(t, inBrowser, "Chrome", t.TempDir())

	st := fetchStatus(t, c, ts)
	if st.DownloadDir != inBrowser || st.DownloadDirFrom != downloadFromBrowser || st.DownloadDirBrowser != "Chrome" || st.DownloadDirSet != "" {
		t.Fatalf("status says updates go to %q from %q (%q, set %q); want the browser's folder %q",
			st.DownloadDir, st.DownloadDirFrom, st.DownloadDirBrowser, st.DownloadDirSet, inBrowser)
	}
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/update/download", "", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download answered %d, want 200", resp.StatusCode)
	}
	ev := waitForDownload(t, srv, "done")
	if filepath.Dir(ev.Path) != inBrowser {
		t.Fatalf("the update landed in %q, want the browser's download folder %q", filepath.Dir(ev.Path), inBrowser)
	}
}

func TestSettingsDownloadDirIsAFolderThatIsThere(t *testing.T) {
	ts, _, c, csrf := downloadServer(t)
	home, _ := os.UserHomeDir()
	system := t.TempDir()
	stubDownloadSources(t, "", "", system)
	good := filepath.Join(home, "Updates")
	if err := os.Mkdir(good, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	save := func(v string) (int, string) {
		t.Helper()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/settings", "application/json",
			jsonBody(map[string]any{"downloadDir": v}))
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var e struct{ Error string }
		_ = json.Unmarshal(b, &e)
		return resp.StatusCode, e.Error
	}

	// `~` is expanded by the server and the stored form is the whole path.
	if code, msg := save(" ~/Updates/ "); code != http.StatusOK {
		t.Fatalf("a folder that is there was refused: %d %s", code, msg)
	}
	if st := fetchStatus(t, c, ts); st.DownloadDirSet != good || st.DownloadDir != good || st.DownloadDirFrom != downloadFromSetting {
		t.Fatalf("after setting ~/Updates the status is set=%q dir=%q from=%q, want %q from the setting",
			st.DownloadDirSet, st.DownloadDir, st.DownloadDirFrom, good)
	}

	// Each refusal says which thing is wrong, and leaves what was stored alone.
	for _, tc := range []struct{ typed, want string }{
		{"Downloads", "whole path"},
		{"../Updates", "whole path"},
		{filepath.Join(home, "no-such-folder"), "could not find"},
		{file, "not a folder"},
	} {
		code, msg := save(tc.typed)
		if code != http.StatusBadRequest || !strings.Contains(msg, tc.want) {
			t.Errorf("typing %q answered %d %q, want 400 saying %q", tc.typed, code, msg, tc.want)
		}
		if st := fetchStatus(t, c, ts); st.DownloadDirSet != good {
			t.Errorf("a refused folder (%q) changed the stored one to %q", tc.typed, st.DownloadDirSet)
		}
	}

	// A stored folder that later goes away is skipped, and the status still says what was set — so
	// the page can say why the folder in use is not the one in the box.
	if err := os.Remove(good); err != nil {
		t.Fatal(err)
	}
	if st := fetchStatus(t, c, ts); st.DownloadDir != system || st.DownloadDirFrom != downloadFromSystem || st.DownloadDirSet != good {
		t.Errorf("with the set folder gone the status is dir=%q from=%q set=%q, want the system folder and the setting still shown",
			st.DownloadDir, st.DownloadDirFrom, st.DownloadDirSet)
	}

	// Clearing the box returns the choice to the browser.
	if code, msg := save(""); code != http.StatusOK {
		t.Fatalf("clearing the folder was refused: %d %s", code, msg)
	}
	if st := fetchStatus(t, c, ts); st.DownloadDirSet != "" || st.DownloadDirFrom != downloadFromSystem {
		t.Errorf("after clearing, set=%q from=%q; want nothing set and the system folder", st.DownloadDirSet, st.DownloadDirFrom)
	}
}

// TestRevealFollowsTheLatestThingTheServerSaid: a file found already there is what reveal shows,
// until a download begins — then it is that download's, and only once it has finished.
func TestRevealFollowsTheLatestThingTheServerSaid(t *testing.T) {
	var d downloadState
	if got := d.revealable(); got != "" {
		t.Fatalf("with nothing downloaded or refused, reveal has %q", got)
	}
	d.sawPresent("/a/already-there")
	if got := d.revealable(); got != "/a/already-there" {
		t.Fatalf("after a file-already-there refusal reveal has %q", got)
	}
	if !d.begin(func() {}, "/b/new", 0) {
		t.Fatal("setup: the slot was not claimed")
	}
	if got := d.revealable(); got != "" {
		t.Errorf("while a new download runs reveal has %q — an earlier refusal's folder, or a file not yet written", got)
	}
	d.finish("done", "")
	if got := d.revealable(); got != "/b/new" {
		t.Errorf("after the download finished reveal has %q, want /b/new", got)
	}
}
