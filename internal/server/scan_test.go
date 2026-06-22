package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The scan endpoint reports active/hidden content for the open document and the
// sanitize endpoint removes it. This locks the wiring (route, auth, JSON shape,
// the ok/reload path) end to end; the per-method removal behaviour itself is
// covered by the pdfops tests. The form fixture carries no active content, so a
// strip is a clean no-op that must still validate and report ok.
func TestScanAndSanitizeWiring(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	resp, err := c.Get(ts.URL + "/api/scan")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scan status = %d, want 200", resp.StatusCode)
	}
	var rep struct {
		Findings []map[string]any `json:"findings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		t.Fatalf("scan response not JSON: %v", err)
	}
	resp.Body.Close()

	resp = write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=strip", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sanitize status = %d, want 200", resp.StatusCode)
	}
	var out sanitizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("sanitize response not JSON: %v", err)
	}
	if !out.Ok {
		t.Error("strip of a clean document should report ok")
	}
}

// The metadata-strip method is wired through the same rail. The form fixture has
// no identifying metadata, so this is a clean no-op that must still report ok
// (the actual /Info //Metadata //ID removal is covered by the pdfops tests).
func TestSanitizeMetadataMethod(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=metadata", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metadata sanitize status = %d, want 200", resp.StatusCode)
	}
	var out sanitizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("sanitize response not JSON: %v", err)
	}
	if !out.Ok {
		t.Error("metadata strip should report ok")
	}
}

// An unknown removal method is a client error, not a silent no-op.
func TestSanitizeUnknownMethod(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=bogus", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown method status = %d, want 400", resp.StatusCode)
	}
}
