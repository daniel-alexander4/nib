package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"nib/internal/instance"
)

// P07.S01 — GET /api/instance, the rendezvous probe.

// TestTheProbeAnswersWhileTheVaultIsLOCKED is the route's whole design decision, and
// the reason it is public rather than behind requireUnlocked.
//
// If the probe needed an unlocked vault, a running-but-locked Nib would read as DEAD: a
// second launch would conclude the record was stale, remove it, take over the rendezvous
// and start a fresh server. The user would end up with two nibs — one holding their
// unlocked session and open documents behind a window that no longer owns the record,
// the other showing a locked first-run screen. Silent, and it costs exactly the state
// this phase exists to preserve.
func TestTheProbeAnswersWhileTheVaultIsLOCKED(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test-version")
	srv.SetInstanceToken("probe-token")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	c := newClient(t)

	// The stimulus, and it is what makes this test about locking at all: the vault must
	// really be locked, or "the probe answered" says nothing about the locked case.
	st := getStatus(t, c, ts.URL)
	if st.State == "ready" {
		t.Fatalf("setup: the vault is already unlocked (state %q) — this test cannot observe the locked case", st.State)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/instance", nil)
	req.Header.Set(instance.HeaderToken, "probe-token")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the probe answered %d while locked, want 200 — a locked instance reads as dead and a second launch will take over its rendezvous", resp.StatusCode)
	}
}

func TestTheProbeRefusesAWrongTokenAndAServerWithNone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test-version")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	c := newClient(t)

	// No token published: this server is not the instance any record names, and saying
	// so is more honest than a friendly 200 that would let a caller treat it as one.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/instance", nil)
	req.Header.Set(instance.HeaderToken, "anything")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a server that published no record answered %d, want 403", resp.StatusCode)
	}

	// With a token, a wrong one is refused — the half that makes a probe mean "this is
	// MY instance" rather than "something is listening on that port".
	srv.SetInstanceToken("right")
	for tok, want := range map[string]int{"right": http.StatusOK, "wrong": http.StatusForbidden, "": http.StatusForbidden} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/instance", nil)
		if tok != "" {
			req.Header.Set(instance.HeaderToken, tok)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("token %q answered %d, want %d", tok, resp.StatusCode, want)
		}
	}
}
