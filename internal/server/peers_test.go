package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func getPeers(t *testing.T, c *http.Client, url string) peersResponse {
	t.Helper()
	resp, err := c.Get(url + "/api/peers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/peers status = %d, want 200", resp.StatusCode)
	}
	var got peersResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

// Listing exposes the user's own fingerprint (identity auto-created), and a peer
// can be pinned, listed, and unpinned — the round-trip the pin-exchange UI needs.
func TestPeersPinListRemove(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	got := getPeers(t, c, ts.URL)
	if len(got.Fingerprint) != 64 {
		t.Fatalf("self fingerprint = %q, want 64 hex chars", got.Fingerprint)
	}
	if len(got.Peers) != 0 {
		t.Fatalf("peers = %d, want 0 initially", len(got.Peers))
	}

	fp := strings.Repeat("ab", 32) // a valid 64-hex (32-byte) fingerprint
	resp := write(t, c, csrf, "POST", ts.URL+"/api/peers/pin", "application/json",
		jsonBody(pinPeerRequest{Fingerprint: fp, Label: "Alice"}))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin status = %d, want 200", resp.StatusCode)
	}

	got = getPeers(t, c, ts.URL)
	if len(got.Peers) != 1 || got.Peers[0].Fingerprint != fp || got.Peers[0].Label != "Alice" {
		t.Fatalf("after pin: %+v, want one Alice peer with fp", got.Peers)
	}

	resp = write(t, c, csrf, "POST", ts.URL+"/api/peers/remove", "application/json",
		jsonBody(removePeerRequest{Fingerprint: fp}))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove status = %d, want 200", resp.StatusCode)
	}
	if got = getPeers(t, c, ts.URL); len(got.Peers) != 0 {
		t.Fatalf("after remove: %d peers, want 0", len(got.Peers))
	}
}

// A fingerprint that isn't 32 bytes of hex must be rejected, not stored.
func TestPeersPinRejectsInvalidFingerprint(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	for _, bad := range []string{"xyz", strings.Repeat("ab", 31), "not hex at all"} {
		resp := write(t, c, csrf, "POST", ts.URL+"/api/peers/pin", "application/json",
			jsonBody(pinPeerRequest{Fingerprint: bad, Label: "x"}))
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("pin %q status = %d, want 400", bad, resp.StatusCode)
		}
	}
	if got := getPeers(t, c, ts.URL); len(got.Peers) != 0 {
		t.Fatalf("invalid pins were stored: %+v", got.Peers)
	}
}

// The spaced/grouped form the UI displays must pin to the same peer as the bare
// hex (parseFingerprint tolerates the display grouping).
func TestPeersPinToleratesGroupedForm(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	bare := strings.Repeat("ab", 32)
	grouped := strings.Join(splitQuads(bare), " ")
	resp := write(t, c, csrf, "POST", ts.URL+"/api/peers/pin", "application/json",
		jsonBody(pinPeerRequest{Fingerprint: grouped, Label: "Bob"}))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pin grouped status = %d, want 200", resp.StatusCode)
	}
	got := getPeers(t, c, ts.URL)
	if len(got.Peers) != 1 || got.Peers[0].Fingerprint != bare {
		t.Fatalf("grouped pin normalized wrong: %+v", got.Peers)
	}
}

func splitQuads(s string) []string {
	var out []string
	for i := 0; i < len(s); i += 4 {
		end := i + 4
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}
