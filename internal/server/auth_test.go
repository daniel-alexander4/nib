package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func getStatus(t *testing.T, c *http.Client, baseURL string) statusResponse {
	t.Helper()
	resp, err := c.Get(baseURL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st statusResponse
	json.NewDecoder(resp.Body).Decode(&st)
	return st
}

func TestStatusProgressesToReady(t *testing.T) {
	ts, _ := startServer(t)
	c := newClient(t)
	if st := getStatus(t, c, ts.URL); st.State != "setup" {
		t.Fatalf("fresh state = %q, want setup", st.State)
	}
	authedClient(t, ts) // enrolls a key, unlocking the vault
	if st := getStatus(t, c, ts.URL); st.State != "ready" {
		t.Errorf("post-enroll state = %q, want ready", st.State)
	}
}

func TestProtectedRouteRequiresUnlock(t *testing.T) {
	ts, _ := startServer(t)
	c := newClient(t)
	resp, err := c.Get(ts.URL + "/api/pdf") // vault not yet unlocked
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("locked /api/pdf = %d, want 401", resp.StatusCode)
	}
}

func TestEnrollRefusedWhenAlreadySetUp(t *testing.T) {
	ts, _ := startServer(t)
	authedClient(t, ts) // first enrollment succeeds
	c := newClient(t)
	body, _ := json.Marshal(enrollRequest{Mode: "create", KeyPath: t.TempDir() + "/k"})
	resp, err := c.Post(ts.URL+"/api/ssh/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("second enroll = %d, want 409", resp.StatusCode)
	}
}

func TestWriteRequiresCSRF(t *testing.T) {
	ts, path := startServer(t)
	c, _ := authedClient(t, ts) // unlocked, but we omit the CSRF header
	body, _ := json.Marshal(openRequest{Path: path})
	resp, err := c.Post(ts.URL+"/api/open", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("write without CSRF = %d, want 403", resp.StatusCode)
	}
}

func TestWriteRejectsForeignOrigin(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/open", jsonBody(openRequest{Path: path}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign-origin write = %d, want 403", resp.StatusCode)
	}
}

func TestEnrollRejectsForeignOrigin(t *testing.T) {
	ts, _ := startServer(t)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/ssh/enroll", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign-origin enroll = %d, want 403", resp.StatusCode)
	}
}

func TestVaultExportImportRoundTrip(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp, err := c.Get(ts.URL + "/api/vault/export")
	if err != nil {
		t.Fatal(err)
	}
	backup, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(backup) == 0 {
		t.Fatal("empty vault export")
	}

	// Re-importing the same vault keeps it unlockable by this machine's key.
	imp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/vault/import", "application/octet-stream", bytes.NewReader(backup))
	var st statusResponse
	json.NewDecoder(imp.Body).Decode(&st)
	imp.Body.Close()
	if imp.StatusCode != http.StatusOK || st.State != "ready" {
		t.Errorf("import status = %d state = %q, want 200/ready", imp.StatusCode, st.State)
	}
	csrf = st.CSRF // import rotates the token; the client refreshes from the response

	// Garbage is rejected.
	bad := write(t, c, csrf, http.MethodPost, ts.URL+"/api/vault/import", "application/octet-stream", bytes.NewReader([]byte("not a vault")))
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Errorf("import garbage = %d, want 400", bad.StatusCode)
	}
}

// A key path is re-read at every unlock, so it has to survive a change of working
// directory. It used to not: enrolling with "~/.ssh/id_ed25519" made a directory
// literally named "~" beside wherever Nib was started, and a bare name landed
// there too — then the next launch, from a different directory, could not find the
// key and the vault would not open.
func TestNormalizeKeyPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	abs := filepath.Join(home, ".ssh", "id_ed25519")

	ok := map[string]string{
		"~/.ssh/id_ed25519": abs,
		abs:                 abs,
		"  " + abs + "  ":   abs, // surrounding space is the user's, not a path
	}
	for in, want := range ok {
		got, err := normalizeKeyPath(in)
		if err != nil {
			t.Errorf("normalizeKeyPath(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeKeyPath(%q) = %q, want %q", in, got, want)
		}
	}

	for _, bad := range []string{"", "   ", "id_ed25519", "keys/id_ed25519", "./id_ed25519", "../id_ed25519"} {
		if got, err := normalizeKeyPath(bad); err == nil {
			t.Errorf("normalizeKeyPath(%q) = %q with no error — a path resolved against the "+
				"working directory will not be found on the next launch", bad, got)
		}
	}
}

// The defect in full: the request was accepted AND a key file appeared beside the
// working directory. Asserting only the status would pass against code that
// rejects for some unrelated reason, so this asserts the file too.
func TestEnrollRejectsRelativeKeyPathAndWritesNothing(t *testing.T) {
	ts, _ := startServer(t)
	c := newClient(t)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(cwd, "id_ed25519")
	os.Remove(stray)
	t.Cleanup(func() { os.Remove(stray); os.Remove(stray + ".pub") })

	body, _ := json.Marshal(enrollRequest{Mode: "create", KeyPath: "id_ed25519"})
	resp, err := c.Post(ts.URL+"/api/ssh/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("enroll(relative) status = %d, want 400", resp.StatusCode)
	}
	if _, err := os.Stat(stray); err == nil {
		t.Errorf("a key was written to %s — relative paths must not reach sshkey.Generate", stray)
	}
	msg, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(msg, []byte("absolute")) {
		t.Errorf("error body = %s, want it to say the path must be absolute", msg)
	}
}

// The second entry point. Fixing only the first-run wizard would leave the same
// defect fully reachable through Manage authorized keys.
func TestAddKeyRejectsRelativeKeyPath(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	cwd, _ := os.Getwd()
	stray := filepath.Join(cwd, "id_ed25519")
	os.Remove(stray)
	t.Cleanup(func() { os.Remove(stray); os.Remove(stray + ".pub") })

	body, _ := json.Marshal(addKeyRequest{Mode: "create", KeyPath: "id_ed25519"})
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/ssh/keys", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("addKey(relative) status = %d, want 400", resp.StatusCode)
	}
	if _, err := os.Stat(stray); err == nil {
		t.Errorf("a key was written to %s via /api/ssh/keys", stray)
	}
}
