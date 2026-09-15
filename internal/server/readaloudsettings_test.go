package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The read-aloud voice and speed — /pending 482. Stored in the vault, carried to the client by
// /api/status, the defaults stored as absence (ViewLayout's rule), and anything that is not a voice
// label or an offered speed refused before it is stored.

func putReadAloud(t *testing.T, c *http.Client, csrf, base string, req settingsRequest) int {
	t.Helper()
	code, _ := postForCode(t, c, csrf, base+"/api/settings", req)
	return code
}

func TestTheReadAloudVoiceAndSpeedSurviveAndReachTheClient(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	voice, rate := "Daniel", 1.25
	if code := putReadAloud(t, c, csrf, ts.URL, settingsRequest{ReadAloudVoice: &voice, ReadAloudRate: &rate}); code != http.StatusOK {
		t.Fatalf("saving a voice and a speed returned %d", code)
	}
	if s := srv.unlockedVault().Settings(); s.ReadAloudVoice != "Daniel" || s.ReadAloudRate != 1.25 {
		t.Errorf("the vault holds voice %q speed %v — the choice does not persist", s.ReadAloudVoice, s.ReadAloudRate)
	}
	resp, err := c.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st struct {
		ReadAloudVoice string  `json:"readAloudVoice"`
		ReadAloudRate  float64 `json:"readAloudRate"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.ReadAloudVoice != "Daniel" || st.ReadAloudRate != 1.25 {
		t.Errorf("/api/status carries voice %q speed %v — the client applies these on boot, so a saved choice never comes back", st.ReadAloudVoice, st.ReadAloudRate)
	}

	// Back to the defaults: both stored as absence.
	empty, normal := "", 1.0
	if code := putReadAloud(t, c, csrf, ts.URL, settingsRequest{ReadAloudVoice: &empty, ReadAloudRate: &normal}); code != http.StatusOK {
		t.Fatalf("returning to the defaults returned %d", code)
	}
	if s := srv.unlockedVault().Settings(); s.ReadAloudVoice != "" || s.ReadAloudRate != 0 {
		t.Errorf("the defaults are stored as voice %q speed %v, want empty and 0 — the default and the absence must be one state", s.ReadAloudVoice, s.ReadAloudRate)
	}
}

func TestAReadAloudSettingThatIsNotAVoiceOrAnOfferedSpeedIsRefused(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	good := "Samantha"
	if code := putReadAloud(t, c, csrf, ts.URL, settingsRequest{ReadAloudVoice: &good}); code != http.StatusOK {
		t.Fatalf("setup: %d", code)
	}
	for name, req := range map[string]settingsRequest{
		"a voice name with a newline": {ReadAloudVoice: ptr("Daniel\nrm")},
		"a voice name too long":       {ReadAloudVoice: ptr(strings.Repeat("v", 201))},
		"a speed too fast":            {ReadAloudRate: ptrF(3)},
		"a speed too slow":            {ReadAloudRate: ptrF(0.25)},
	} {
		if code := putReadAloud(t, c, csrf, ts.URL, req); code != http.StatusBadRequest {
			t.Errorf("%s returned %d, want 400", name, code)
		}
	}
	if s := srv.unlockedVault().Settings(); s.ReadAloudVoice != "Samantha" || s.ReadAloudRate != 0 {
		t.Errorf("a refused setting changed the vault: voice %q speed %v", s.ReadAloudVoice, s.ReadAloudRate)
	}
	// A name at the bound is a voice.
	edge := strings.Repeat("v", 200)
	if code := putReadAloud(t, c, csrf, ts.URL, settingsRequest{ReadAloudVoice: &edge}); code != http.StatusOK {
		t.Errorf("a 200-byte voice name returned %d, want 200", code)
	}
}

func ptr(s string) *string    { return &s }
func ptrF(f float64) *float64 { return &f }
