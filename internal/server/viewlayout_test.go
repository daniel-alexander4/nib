package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The view layout setting — `/pending`-less, from Dan's 2026-09-10 /discuss round.
//
// # The rule these drive, and it is not a style choice
//
// `vault.Settings` is entirely `omitempty`, and `Contents.Version`'s own doc records what that
// costs: an older build opens a payload, does not know a key, and **drops it on re-save**. So for
// every setting the DEFAULT and the ABSENCE have to be one state on disk. The default layout is
// `pages`, therefore `pages` is stored as **empty** and only `continuous` is written down.
//
// Store the literal "pages" instead and the rule inverts silently: a user on the default, opened
// once by a build without the field, comes back on whatever the default is *then* — which is fine
// today and is a trap the moment the default moves.

// putLayout posts one layout and returns the code and body.
func putLayout(t *testing.T, c *http.Client, csrf, base, layout string) (int, string) {
	t.Helper()
	return postForCode(t, c, csrf, base+"/api/settings", settingsRequest{ViewLayout: &layout})
}

func TestTheDefaultLayoutIsStoredAsAbsenceAndContinuousIsStoredAsItself(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	// SETUP: nothing stored yet, so the assertions below are about what this route WRITES rather
	// than about a field that happened to be empty already.
	if got := storedLayout(t, srv); got != "" {
		t.Fatalf("setup: a fresh vault already holds viewLayout %q", got)
	}

	if code, body := putLayout(t, c, csrf, ts.URL, "continuous"); code != http.StatusOK {
		t.Fatalf("saving continuous returned %d: %s", code, body)
	}
	if got := storedLayout(t, srv); got != "continuous" {
		t.Errorf("the vault holds viewLayout %q after saving continuous — the setting does not "+
			"persist, so every restart puts the user back on a layout they turned off", got)
	}

	if code, body := putLayout(t, c, csrf, ts.URL, "pages"); code != http.StatusOK {
		t.Fatalf("saving pages returned %d: %s", code, body)
	}
	if got := storedLayout(t, srv); got != "" {
		t.Errorf("the vault holds viewLayout %q after returning to the default, want empty — the "+
			"default and the absence have to be one state, because every field in Settings is "+
			"omitempty and a build that does not know the key drops it on re-save", got)
	}
}

func TestAnUnknownLayoutIsRefusedRatherThanStored(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	if code, _ := putLayout(t, c, csrf, ts.URL, "spreads"); code != http.StatusBadRequest {
		t.Errorf("an unknown layout returned %d, want 400 — a layout name this build does not "+
			"implement would be stored, read back, and applied as nothing at all", code)
	}
	if got := storedLayout(t, srv); got != "" {
		t.Errorf("the refused layout was stored anyway (%q)", got)
	}
}

// storedLayout reads the value out of the vault, which is the only place it lives.
func storedLayout(t *testing.T, srv *Server) string {
	t.Helper()
	v := srv.unlockedVault()
	if v == nil {
		t.Fatal("the server holds no unlocked vault")
	}
	return v.Settings().ViewLayout
}

// TestTheStatusRouteCarriesTheLayoutToTheClient — a setting the client cannot read is a row in a
// vault and nothing else.
func TestTheStatusRouteCarriesTheLayoutToTheClient(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)

	if code, body := putLayout(t, c, csrf, ts.URL, "continuous"); code != http.StatusOK {
		t.Fatalf("setup: %d %s", code, body)
	}
	resp, err := c.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st struct {
		ViewLayout string `json:"viewLayout"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.ViewLayout != "continuous" {
		t.Errorf("/api/status reported viewLayout %q — the client applies the layout from this "+
			"field on boot, so an empty one means the saved setting never comes back", st.ViewLayout)
	}
}
