package server

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// `/pending 685`, ADR-053: no local process gets Nib's credentials except through a launch key.

// TestStatusNeverCarriesTheToken is the defect as filed: `GET /api/status` answers any loopback
// caller — curl sends neither `Sec-Fetch-Site` nor `Origin`, and `originIsLoopback` passes that —
// and it used to put the CSRF token in the body once the vault was unlocked. Read raw, not through
// `statusResponse`, so a field re-added under any Go name is still seen by its JSON key.
func TestStatusNeverCarriesTheToken(t *testing.T) {
	ts, _ := startServer(t)
	_, csrf := authedClient(t, ts)
	// A caller with no session and no browser headers: what any other process on the machine is.
	res, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(raw), `"state":"ready"`) {
		t.Fatalf("setup: the vault is not unlocked, so the token would be absent anyway: %s", raw)
	}
	if strings.Contains(string(raw), `"csrf"`) || strings.Contains(string(raw), csrf) {
		t.Fatalf("GET /api/status hands the CSRF token to a header-less caller: %s", raw)
	}
}

// TestACallerWithoutTheSessionIsRefusedOnEveryMethod — the reads were half the defect:
// `requireUnlocked` checked the token on writes only, so `/api/docs`, `/api/pdf` and
// `/api/listdir` answered anyone. A client that never traded a key is refused a GET and a write.
func TestACallerWithoutTheSessionIsRefusedOnEveryMethod(t *testing.T) {
	ts, _ := startServer(t)
	_, csrf := authedClient(t, ts) // unlocks the vault; this client is NOT the one below
	stranger := &http.Client{}
	for _, route := range []string{"/api/docs", "/api/listdir", "/api/peers", "/api/vault/export", "/api/launch"} {
		res, err := stranger.Get(ts.URL + route)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s without a session = %d, want 403", route, res.StatusCode)
		}
	}
	res := write(t, stranger, "", http.MethodPost, ts.URL+"/api/settings", "application/json", strings.NewReader("{}"))
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("POST /api/settings without a session = %d, want 403", res.StatusCode)
	}
	// And the counter-pin: the token itself still authenticates, so the refusals above are about
	// the missing credential and not a route that refuses everybody.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/docs", nil)
	req.Header.Set("X-CSRF-Token", csrf)
	res, err := stranger.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("GET /api/docs WITH the token = %d, want 200", res.StatusCode)
	}
}

// TestALaunchKeyTradesOnce: the fragment stays in browser history, so a spent key must open nothing.
// And the cookie the trade sets authenticates a GET on its own, which is what a sub-resource load
// (`<img src>`, pdf.js, a download) has to rely on.
func TestALaunchKeyTradesOnce(t *testing.T) {
	ts, srv := startServerWith(t)
	key := srv.MintLaunchKey()
	trade := func(c *http.Client, k string) int {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", nil)
		req.Header.Set(headerLaunch, k)
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	first, second := newClient(t), newClient(t)
	if code := trade(first, key); code != http.StatusOK {
		t.Fatalf("first trade = %d, want 200", code)
	}
	if code := trade(second, key); code != http.StatusForbidden {
		t.Fatalf("second trade of the same key = %d, want 403", code)
	}
	if code := trade(second, "never-minted"); code != http.StatusForbidden {
		t.Fatalf("trade of an unminted key = %d, want 403", code)
	}
	for c, want := range map[*http.Client]int{first: http.StatusOK, second: http.StatusForbidden} {
		res, err := c.Get(ts.URL + "/api/launch")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("GET /api/launch with the cookie a trade did (%d) or did not (403) set = %d", want, res.StatusCode)
		}
	}
}

// TestAWindowlessHandoffGetsAKeyForThisInstance: "open Nib again" is how a tab without the session
// gets in, so a launch with no document must be given a window onto the RUNNING instance — it used
// to be a 400, which starts a second Nib and strands the user's documents in the first — and the key
// it is given must actually trade.
func TestAWindowlessHandoffGetsAKeyForThisInstance(t *testing.T) {
	ts, srv := startServerWith(t)
	srv.SetHandoffSecret("secret")
	c := newClient(t)
	code, out := handoffTo(t, ts, c, "secret", "")
	if code != http.StatusOK || out.Result != "window" || out.Launch == "" {
		t.Fatalf("a hand-off with no document = %d/%q launch=%v, want 200/window with a key", code, out.Result, out.Launch != "")
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", bytes.NewReader(nil))
	req.Header.Set(headerLaunch, out.Launch)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("the hand-off's key does not trade: %d", res.StatusCode)
	}
	// Without the secret there is no key: the hand-off is not a second way to mint one.
	if code, out := handoffTo(t, ts, c, "", ""); code != http.StatusForbidden || out.Launch != "" {
		t.Fatalf("a hand-off without the secret = %d, launch=%v; want 403 and no key", code, out.Launch != "")
	}
}

// TestAnUntradedKeyExpires: a key minted for a window that never opened must not stay good for the
// life of the process.
func TestAnUntradedKeyExpires(t *testing.T) {
	ts, srv := startServerWith(t)
	key := srv.MintLaunchKey()
	srv.mu.Lock()
	srv.launchKeys[len(srv.launchKeys)-1].minted = time.Now().Add(-launchKeyTTL - time.Second)
	srv.mu.Unlock()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", nil)
	req.Header.Set(headerLaunch, key)
	res, err := newClient(t).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("an expired key traded: %d, want 403", res.StatusCode)
	}
}
