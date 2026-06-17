package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fetchStatus reads /api/status, the canonical read-back for saved settings.
func fetchStatus(t *testing.T, c *http.Client, ts *httptest.Server) statusResponse {
	t.Helper()
	resp, err := c.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

// A valid toolbar style is persisted and reported back via /api/status.
func TestSettingsPersistsToolbarStyle(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp := write(t, c, csrf, "POST", ts.URL+"/api/settings", "application/json",
		jsonBody(map[string]any{"toolbarStyle": "both"}))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings status = %d, want 200", resp.StatusCode)
	}
	if st := fetchStatus(t, c, ts); st.ToolbarStyle != "both" {
		t.Fatalf("toolbarStyle = %q, want both", st.ToolbarStyle)
	}
}

// An out-of-allowlist toolbar style is rejected and leaves the vault unchanged.
func TestSettingsRejectsInvalidToolbarStyle(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp := write(t, c, csrf, "POST", ts.URL+"/api/settings", "application/json",
		jsonBody(map[string]any{"toolbarStyle": "garbage"}))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("settings status = %d, want 400", resp.StatusCode)
	}
	if st := fetchStatus(t, c, ts); st.ToolbarStyle != "menus" {
		t.Fatalf("toolbarStyle = %q, want unchanged default menus", st.ToolbarStyle)
	}
}

// A valid appearance is persisted and reported back via /api/status.
func TestSettingsPersistsAppearance(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp := write(t, c, csrf, "POST", ts.URL+"/api/settings", "application/json",
		jsonBody(map[string]any{"appearance": "light"}))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings status = %d, want 200", resp.StatusCode)
	}
	if st := fetchStatus(t, c, ts); st.Appearance != "light" {
		t.Fatalf("appearance = %q, want light", st.Appearance)
	}
}

// An out-of-allowlist appearance is rejected and leaves the vault unchanged.
func TestSettingsRejectsInvalidAppearance(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp := write(t, c, csrf, "POST", ts.URL+"/api/settings", "application/json",
		jsonBody(map[string]any{"appearance": "garbage"}))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("settings status = %d, want 400", resp.StatusCode)
	}
	if st := fetchStatus(t, c, ts); st.Appearance != "dark" {
		t.Fatalf("appearance = %q, want unchanged default dark", st.Appearance)
	}
}

// A partial update touches only the field it carries: toggling the update check
// must not reset a previously-saved toolbar style.
func TestSettingsPartialUpdatePreservesOtherFields(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	r1 := write(t, c, csrf, "POST", ts.URL+"/api/settings", "application/json",
		jsonBody(map[string]any{"toolbarStyle": "toolbar"}))
	r1.Body.Close()
	r2 := write(t, c, csrf, "POST", ts.URL+"/api/settings", "application/json",
		jsonBody(map[string]any{"checkUpdatesOnStartup": false}))
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("settings status = %d, want 200", r2.StatusCode)
	}
	st := fetchStatus(t, c, ts)
	if st.AutoUpdate {
		t.Error("autoUpdate = true, want false after disabling the startup check")
	}
	if st.ToolbarStyle != "toolbar" {
		t.Errorf("toolbarStyle = %q, want toolbar preserved across the partial update", st.ToolbarStyle)
	}
}
