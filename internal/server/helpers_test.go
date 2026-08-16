package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/testpdf"
)

// startServer returns a running test server backed by an empty (setup-needed)
// vault directory, plus a path to a sample form PDF on disk.
func startServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	// Isolate ~/.ssh so the builtin-key auto-setup can't open or create a vault
	// from the host's real keys — fresh state must stay "setup".
	t.Setenv("HOME", t.TempDir())
	data, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(t.TempDir(), "form.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test").Handler())
	t.Cleanup(ts.Close)
	return ts, pdfPath
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{}
}

// authedClient enrolls a freshly generated SSH key (first run), which unlocks the
// vault, and returns a client plus the CSRF token to send on writes.
func authedClient(t *testing.T, ts *httptest.Server) (*http.Client, string) {
	t.Helper()
	c := newClient(t)
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	body, _ := json.Marshal(enrollRequest{Mode: "create", KeyPath: keyPath})
	resp, err := c.Post(ts.URL+"/api/ssh/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enroll status = %d, want 200", resp.StatusCode)
	}
	var st statusResponse
	json.NewDecoder(resp.Body).Decode(&st)
	if st.State != "ready" || st.CSRF == "" {
		t.Fatalf("enroll state = %q csrf=%q, want ready with csrf", st.State, st.CSRF)
	}
	return c, st.CSRF
}

// write issues a state-changing request with the CSRF header set.
func write(t *testing.T, c *http.Client, csrf, method, url, contentType string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// jsonBody marshals v to a reader for request bodies.
func jsonBody(v any) *bytes.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// openTestServer returns a Server holding one open document, built through the
// real setDoc path rather than by hand.
//
// Three tests used to write `&Server{doc: …}` directly. With the registry that no
// longer compiles, and hand-assembling docs/activeID/nextSeq/epoch in a test would
// be a second, silently-drifting copy of setDoc's invariants — including the id
// rules ADR-001 turns on. Going through setDoc means these tests exercise the same
// construction production does, and get a real id for free.
func openTestServer(t *testing.T, pdf []byte) *Server {
	t.Helper()
	s := &Server{epoch: "test-epoch"}
	s.setDoc(&document{data: pdf})
	return s
}
