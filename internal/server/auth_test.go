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

// The repoint route is a WRITE, and it is refused from a foreign origin like every other
// pre-unlock write.
//
// It runs before the vault opens, so there is no CSRF token to demand and a loopback
// Origin is the only guard available — the same shape as enroll and unlock. Asserted
// separately rather than assumed from "it uses requirePublicLoopback", because a route
// registered with the wrong wrapper looks identical at the call site.
func TestRepointRejectsForeignOrigin(t *testing.T) {
	ts, _ := startServerWith(t)
	body := []byte(`{"keyPath":"/tmp/nowhere/id_ed25519"}`)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/ssh/repoint", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin repoint = %d, want 403", resp.StatusCode)
	}
}

// A relative or ~-prefixed key path is refused by the RECOVERY route too.
//
// This is the point most worth pinning: the state being recovered from is a vault holding
// exactly such a path, so a recovery that accepted one would write back the very thing
// that caused it — and report success.
func TestRepointRejectsUnstableKeyPath(t *testing.T) {
	ts, _ := startServerWith(t)
	for _, bad := range []string{"id_ed25519", "./keys/id_ed25519", ""} {
		body, _ := json.Marshal(map[string]string{"keyPath": bad})
		resp, err := http.Post(ts.URL+"/api/ssh/repoint", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("repoint with key path %q = %d, want 400 — accepting it would write back the unstable path this route exists to repair", bad, resp.StatusCode)
		}
	}
}

// TestASubResourceGetCannotReachTheVault.
//
// `requirePublicLoopback` was put on `GET /api/status` because *"the method-based guard let
// any web page the user had open trigger first-run vault creation with a plain cross-site
// request"* — handleStatus calls ensureUnlocked, which can run vault.AutoSetup. It stopped
// `fetch()` and nothing else: browsers send **no Origin** on sub-resource GETs, so an
// `<img src="http://127.0.0.1:PORT/api/status">` on any open page passed the guard.
//
// The population is what matters here. A test that only drove `Origin: https://evil` would
// have passed against the shipped code — that case was already refused. The case that was
// not refused is the one with no Origin at all.
func TestASubResourceGetCannotReachTheVault(t *testing.T) {
	for _, c := range []struct {
		name     string
		headers  map[string]string
		wantPass bool
		why      string
	}{
		{
			name:     "img tag on a hostile page",
			headers:  map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "no-cors"},
			wantPass: false,
			why:      "no Origin is sent for a sub-resource GET, so this passed the shipped guard",
		},
		{
			name:     "script tag on a hostile page",
			headers:  map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Dest": "script"},
			wantPass: false,
			why:      "same shape, different element",
		},
		{
			name:     "cross-site fetch, which the shipped guard did catch",
			headers:  map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"},
			wantPass: false,
			why:      "the positive control: this must stay refused",
		},
		// The controls. A guard that refuses everything is an outage, not a fix.
		{
			name:     "Nib's own UI",
			headers:  map[string]string{"Sec-Fetch-Site": "same-origin"},
			wantPass: true,
			why:      "an ordinary same-origin GET sends no Origin either — requiring Origin would break the app",
		},
		{
			name:     "the user typing the URL",
			headers:  map[string]string{"Sec-Fetch-Site": "none"},
			wantPass: true,
			why:      "a user-initiated navigation",
		},
		{
			name:     "a client that sends no metadata at all",
			headers:  nil,
			wantPass: true,
			why:      "curl and the CLI — falls back to the Origin rule, unchanged",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			if got := originIsLoopback(req); got != c.wantPass {
				t.Errorf("originIsLoopback = %v, want %v — %s", got, c.wantPass, c.why)
			}
		})
	}
}
