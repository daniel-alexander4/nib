package server

import (
	"net/http"
	"reflect"
	"testing"

	"nib/internal/vault"
)

// `/pending 930` — pinned colours and each tool's remembered style persist, are bounded on the way in, and come back
// through status.

func setSettings(t *testing.T, ts, csrf string, c *http.Client, body map[string]any) {
	t.Helper()
	resp := write(t, c, csrf, "POST", ts+"/api/settings", "application/json", jsonBody(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings status = %d, want 200", resp.StatusCode)
	}
}

func TestPinnedColoursPersistAreNormalisedAndCapped(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	setSettings(t, ts.URL, csrf, c, map[string]any{"pinnedColors": []string{
		"#FF0000", "#ff0000", "red", "#00ff00", "#0000ff", "#111111", "#222222", "#333333", "#444444", "#555555", "#666666",
	}})
	got := fetchStatus(t, c, ts).PinnedColors
	want := []string{"#ff0000", "#00ff00", "#0000ff", "#111111", "#222222", "#333333", "#444444", "#555555"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pinnedColors = %v, want %v (lowercased, deduped, junk dropped, capped at 8)", got, want)
	}
	// The recent list is a different list: setting one does not touch the other.
	setSettings(t, ts.URL, csrf, c, map[string]any{"recentHighlightColors": []string{"#abcdef"}})
	if got := fetchStatus(t, c, ts).PinnedColors; !reflect.DeepEqual(got, want) {
		t.Errorf("setting the recent colours changed the pins: %v", got)
	}
	setSettings(t, ts.URL, csrf, c, map[string]any{"pinnedColors": []string{}})
	if got := fetchStatus(t, c, ts).PinnedColors; len(got) != 0 {
		t.Errorf("an empty list did not clear the pins: %v", got)
	}
}

func TestMarkupStylesPersistAndAreBoundedFieldByField(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	setSettings(t, ts.URL, csrf, c, map[string]any{"markupStyles": map[string]any{
		"INK":       map[string]any{"color": "#0000FF", "thickness": 9, "opacity": 0.5},
		"FREETEXT":  map[string]any{"color": "#ff0000", "size": 18},
		"border":    map[string]any{"color": "#00aa00"},
		"HIGHLIGHT": map[string]any{"color": "not a colour", "thickness": 500, "opacity": 7, "size": 1}, // every field out of range
		"scissors":  map[string]any{"color": "#123456"},                                                 // not a tool
	}})
	got := fetchStatus(t, c, ts).MarkupStyles
	want := map[string]vault.MarkupStyle{
		"INK":      {Color: "#0000ff", Thickness: 9, Opacity: 0.5},
		"FREETEXT": {Color: "#ff0000", Size: 18},
		"border":   {Color: "#00aa00"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("markupStyles = %+v, want %+v (an unknown tool dropped, each bad field dropped, an emptied entry dropped whole)", got, want)
	}
	setSettings(t, ts.URL, csrf, c, map[string]any{"markupStyles": map[string]any{}})
	if got := fetchStatus(t, c, ts).MarkupStyles; len(got) != 0 {
		t.Errorf("an empty map did not clear the styles: %+v", got)
	}
}
