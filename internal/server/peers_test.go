package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"nib/internal/pairing"
	"nib/internal/vault"
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

// TestPeersCarryTheirSixWordName covers P01.S02's first clause: the payload gains the
// name beside the fingerprint, and nothing existing is removed.
func TestPeersCarryTheirSixWordName(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	peerFP := bytes.Repeat([]byte{0xAB}, 32)
	resp := write(t, c, csrf, "POST", ts.URL+"/api/peers/pin", "application/json",
		strings.NewReader(`{"fingerprint":"`+hex.EncodeToString(peerFP)+`","label":"Ada"}`))
	resp.Body.Close()

	got := getPeers(t, c, ts.URL)

	// Nothing existing removed — the fingerprint is still the load-bearing value.
	if len(got.Fingerprint) != 64 {
		t.Errorf("own fingerprint is %q, want 64 hex characters", got.Fingerprint)
	}
	if len(got.Peers) != 1 || got.Peers[0].Label != "Ada" {
		t.Fatalf("peers = %+v, want one peer labelled Ada", got.Peers)
	}

	// The names, checked against a derivation this test does itself. Comparing the
	// payload to pairing.Name() rather than to a literal keeps the test honest about
	// what it knows: it asserts the server derived the name from THAT fingerprint, not
	// that six particular words came back.
	ownFP, err := hex.DecodeString(got.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	wantOwn, err := pairing.Name(ownFP)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != wantOwn {
		t.Errorf("own name = %q, want %q", got.Name, wantOwn)
	}
	wantPeer, err := pairing.Name(peerFP)
	if err != nil {
		t.Fatal(err)
	}
	if got.Peers[0].Name != wantPeer {
		t.Errorf("peer name = %q, want %q", got.Peers[0].Name, wantPeer)
	}
	if got.Name == got.Peers[0].Name {
		t.Error("the user and the peer share a name — the derivation is not reading the fingerprint")
	}
}

// TestPeerNameIsNeverStored is P01.S02's second clause, and the clause had to be
// re-specified twice before it could fail at all.
//
// As written in the plan it was "deriving twice yields the same words" — which is true
// whether or not the name is stored, so it could not see the property it named. The
// obvious replacement, reading the vault file and asserting the words are absent, is
// WORSE: the vault is encrypted at rest, so nothing appears in it in plaintext and the
// assertion is satisfied by the encryption rather than by the design. It would pass with
// the name stored.
//
// So the property is asserted where it is actually decidable:
//
//  1. the payload's name equals a derivation this test performs from the fingerprint —
//     which catches a stored name that has DRIFTED from what the key now encodes, the
//     failure that matters (a user shown a name their key no longer produces); and
//  2. the vault's own persisted type has no field that could hold one — which catches
//     the storage being introduced at all, including on the day it still agrees.
//
// Assertion 1 alone cannot see storage that agrees with the derivation, which is exactly
// why 2 exists and why it is a structural check rather than a behavioural one.
func TestPeerNameIsNeverStored(t *testing.T) {
	pp := reflect.TypeOf(vault.PinnedPeer{})
	for i := 0; i < pp.NumField(); i++ {
		if n := pp.Field(i).Name; n != "Fingerprint" && n != "Label" {
			t.Errorf("vault.PinnedPeer gained a %q field. The six-word name is DERIVED from the "+
				"fingerprint on every read (D3) and must not be persisted: a stored name outlives "+
				"the derivation that produced it, so a wordlist or encoding change shows the user a "+
				"name their key no longer encodes, with nothing to say the two disagree.", n)
		}
	}
}
