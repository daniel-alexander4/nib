package server

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The main-menu visibility switch (ADR-036). Dan, 2026-09-16: *"each main menu item is
// enable/disable-able in advanced settings. If an item is disabled in the advanced settings, it
// should not show in the main menu."*
//
// ── The half these tests own ─────────────────────────────────────────────────
// This file proves the STORE: what the route accepts, what it refuses, and that the answer comes
// back across a read. The hiding itself is `test/jsdom/modevisibility.test.mjs`, which has a DOM.
//
// Split for the reason `advanced_test.go` and `advanced.test.mjs` are — except that here the split
// is unusually clean, because this switch has NOTHING to enforce at a door. It is presentational by
// design (ADR-036), so the failure that would make it a lie is the opposite one: a tab that stays
// in the menu after being switched off, which only a DOM can see.

// TestEveryHideableModeIsARealTab holds the Go whitelist against the markup that declares the
// modes — the same two-way comparison `theme.test.mjs` runs for the card hues, for the same reason:
// each disagreement fails silently and differently. A mode in the markup and missing here is
// refused by the route with no symptom anywhere else; a mode here and missing from the markup is a
// switch for something that does not exist.
func TestEveryHideableModeIsARealTab(t *testing.T) {
	markup, err := os.ReadFile(filepath.Join("..", "..", "web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	tabs := map[string]bool{}
	for _, m := range regexp.MustCompile(`<button class="modetab" data-tab="([a-z]+)">`).
		FindAllStringSubmatch(string(markup), -1) {
		tabs[m[1]] = true
	}
	if len(tabs) < 5 {
		t.Fatalf("found %d mode tabs in the markup; this app has seven. The scan is broken, so every "+
			"assertion below would be about nothing.", len(tabs))
	}
	if !tabs["settings"] {
		t.Fatal("the markup declares no `settings` tab, so the exemption asserted below is not the " +
			"exemption this test thinks it is")
	}

	for m := range hideableModes {
		if !tabs[m] {
			t.Errorf("hideableModes offers to hide %q, and no tab is called that — the Settings card "+
				"would carry a switch for a mode that does not exist", m)
		}
	}
	for m := range tabs {
		if m == "settings" {
			continue
		}
		if !hideableModes[m] {
			t.Errorf("the markup declares the %q tab and hideableModes does not list it, so the route "+
				"REFUSES every request naming it. The Settings card would offer a box that always errors", m)
		}
	}

	// The exemption, asserted rather than assumed: the card that un-hides a mode is inside Settings,
	// so hiding Settings is the one choice a user cannot undo from the UI.
	if hideableModes["settings"] {
		t.Error("`settings` is hideable — the switch that would undo it lives in the mode it hides, " +
			"so the only way back would be editing the vault")
	}
}

func TestHiddenModesRoundTripsAndRefusesWhatIsNotAMode(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	post := func(v any) int {
		t.Helper()
		resp := write(t, c, csrf, "POST", ts.URL+"/api/settings", "application/json", jsonBody(v))
		defer resp.Body.Close()
		return resp.StatusCode
	}
	hidden := func() []string {
		t.Helper()
		got := append([]string(nil), fetchStatus(t, c, ts).HiddenModes...)
		sort.Strings(got)
		return got
	}

	// Setup: nothing is hidden on a vault that has never been asked, or every assertion below is
	// about a state the user did not reach.
	if got := hidden(); len(got) != 0 {
		t.Fatalf("setup: a fresh vault already hides %v", got)
	}

	if code := post(map[string]any{"hiddenModes": []string{"secure", "collaborate"}}); code != http.StatusOK {
		t.Fatalf("hiding two modes answered %d, want 200", code)
	}
	if got := strings.Join(hidden(), ","); got != "collaborate,secure" {
		t.Errorf("status reports hiddenModes=%q after hiding secure and collaborate — the menu would "+
			"come back from a restart showing tabs the user switched off", got)
	}

	// **Refused, not filtered.** A silently dropped id stores a set the user did not choose while
	// answering 200, which is the failure `viewLayout`'s own refusal branch exists to prevent.
	if code := post(map[string]any{"hiddenModes": []string{"secure", "nosuchmode"}}); code != http.StatusBadRequest {
		t.Errorf("a request naming a mode that does not exist answered %d, want 400", code)
	}
	if code := post(map[string]any{"hiddenModes": []string{"settings"}}); code != http.StatusBadRequest {
		t.Errorf("hiding Settings answered %d, want 400 — the switch that would undo it is inside the "+
			"mode it hides", code)
	}
	if got := strings.Join(hidden(), ","); got != "collaborate,secure" {
		t.Errorf("a refused request still moved the stored set to %q — the handler wrote before it "+
			"finished validating", got)
	}

	// Empty is stored as absence: nothing hidden and never asked are one state, both meaning every
	// mode shows.
	if code := post(map[string]any{"hiddenModes": []string{}}); code != http.StatusOK {
		t.Fatalf("unticking the last box answered %d, want 200", code)
	}
	if got := hidden(); len(got) != 0 {
		t.Errorf("after unticking everything status still reports %v", got)
	}

	// A duplicate is not an error; it is one choice said twice.
	if code := post(map[string]any{"hiddenModes": []string{"edit", "edit"}}); code != http.StatusOK {
		t.Fatalf("a duplicated id answered %d, want 200", code)
	}
	if got := hidden(); len(got) != 1 || got[0] != "edit" {
		t.Errorf("a duplicated id stored %v, want exactly one entry", got)
	}

	// A write naming something else must leave this field alone: it is a pointer, and an absent
	// field is what a client sends when it is changing something other than the menu.
	if code := post(map[string]any{"appearance": "light"}); code != http.StatusOK {
		t.Fatalf("changing the theme answered %d, want 200", code)
	}
	if got := hidden(); len(got) != 1 || got[0] != "edit" {
		t.Errorf("changing the theme moved the hidden set to %v — a partial update clobbered a "+
			"sibling field", got)
	}
}
