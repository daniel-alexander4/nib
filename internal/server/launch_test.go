package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// `/pending 685` (ADR-053) and `/pending 704` (ADR-054): no local process gets Nib's token except
// through a launch key, and no route but the three that carry their own secret answers without it.

// stranger is what any other process on the machine is: no token, no browser headers.
func stranger(t *testing.T, method, url string, body io.Reader) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, body)
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// TestStatusNeverCarriesTheToken is the defect as first filed: `GET /api/status` answered any loopback
// caller with the token in its body. It now refuses a stranger outright, and even the page's own
// answer carries no token — read raw, so a field re-added under any Go name is seen by its JSON key.
func TestStatusNeverCarriesTheToken(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	res, err := c.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(raw), `"state":"ready"`) {
		t.Fatalf("setup: the vault is not unlocked, so the token would be absent anyway: %s", raw)
	}
	if strings.Contains(string(raw), `"csrf"`) || strings.Contains(string(raw), csrf) {
		t.Fatalf("GET /api/status carries the token: %s", raw)
	}
	if code, body := stranger(t, http.MethodGet, ts.URL+"/api/status", nil); code != http.StatusForbidden || strings.Contains(body, csrf) {
		t.Fatalf("GET /api/status to a stranger = %d %s, want 403 and no token", code, body)
	}
}

// routeLine matches a registration in `Handler`: `mux.HandleFunc("METHOD /path", …)` or
// `mux.Handle("…", …)`.
var routeLine = regexp.MustCompile(`mux\.Handle(?:Func)?\("([^"]+)"`)

// routeOwnSecret are the routes that authenticate by their OWN secret, and the page's static files.
// Every other route must refuse a stranger with the session's 403.
var routeOwnSecret = map[string]string{
	"POST /api/launch":  "the launch key is its credential",
	"GET /api/instance": "the probe token is its credential (internal/instance)",
	"POST /api/handoff": "the hand-off secret is its credential (ADR-006)",
	"GET /legal/":       "static licence texts",
	"/":                 "the page itself",
}

// TestEveryRouteIsBehindTheSessionOrNamed is the census the one door needs (ADR-009; the P08
// phase-close review, R3-4): every route `Handler` registers is driven as a stranger and must answer
// 403 `no session`, unless it is named above. Read from the source so a route added tomorrow is in the
// population without anyone remembering to list it; driven, not read, so a wrapper that looks right
// and is not is caught too. `/api/quit` escaped ADR-053 exactly this way.
func TestEveryRouteIsBehindTheSessionOrNamed(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	ts, _ := startServer(t)
	authedClient(t, ts) // an unlocked vault, so a refusal below is the credential and not the lock
	seen := map[string]bool{}
	checked := 0
	for _, m := range routeLine.FindAllStringSubmatch(string(src), -1) {
		pat := m[1]
		seen[pat] = true
		if _, own := routeOwnSecret[pat]; own {
			continue
		}
		method, path, ok := strings.Cut(pat, " ")
		if !ok {
			t.Errorf("route %q has no method, so every method reaches it — name it or give it one", pat)
			continue
		}
		path = regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "x")
		code, body := stranger(t, method, ts.URL+path, strings.NewReader("{}"))
		checked++
		if code != http.StatusForbidden || !strings.Contains(body, "no session") {
			t.Errorf("%s answered a stranger %d %q — every route but the named few requires the session", pat, code, strings.TrimSpace(body))
		}
	}
	// The stimulus: the scan found the table. A pattern that matched nothing would pass everything.
	if checked < 100 {
		t.Fatalf("stimulus: only %d route(s) found in server.go — the census is not reading the route table", checked)
	}
	for pat := range routeOwnSecret {
		if !seen[pat] {
			t.Errorf("routeOwnSecret names %q, which server.go no longer registers — remove it", pat)
		}
	}
}

// TestTheTokenIsTheOnlyCredential: the header authenticates any method; the `auth` query parameter
// authenticates a GET (what `<img>`, pdf.js, EventSource and a download can carry) and never a write;
// and the trade sets NO cookie — cookies are not isolated by port, so one on 127.0.0.1 reached every
// other loopback server the browser visited (the P08 phase-close review, R3-5).
func TestTheTokenIsTheOnlyCredential(t *testing.T) {
	ts, _ := startServer(t)
	_, csrf := authedClient(t, ts)
	do := func(method, url, header string) int {
		req, _ := http.NewRequest(method, url, strings.NewReader("{}"))
		if header != "" {
			req.Header.Set("X-CSRF-Token", header)
		}
		res, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := do(http.MethodGet, ts.URL+"/api/docs", csrf); code != http.StatusOK {
		t.Errorf("GET with the token in the header = %d, want 200", code)
	}
	if code := do(http.MethodGet, ts.URL+"/api/docs?auth="+csrf, ""); code != http.StatusOK {
		t.Errorf("GET with the token in the query = %d, want 200", code)
	}
	if code := do(http.MethodGet, ts.URL+"/api/docs?auth=wrong", ""); code != http.StatusForbidden {
		t.Errorf("GET with a wrong query token = %d, want 403", code)
	}
	if code := do(http.MethodPost, ts.URL+"/api/settings?auth="+csrf, ""); code != http.StatusForbidden {
		t.Errorf("a WRITE with the token in the query only = %d, want 403 — the query form grants reads", code)
	}
	srv, _ := testServers.Load(ts.URL)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", nil)
	req.Header.Set(headerLaunch, srv.(*Server).MintLaunchKey())
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("setup: the trade answered %d", res.StatusCode)
	}
	if got := res.Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("the trade sets a cookie %v — a cookie on 127.0.0.1 is sent to every other loopback port", got)
	}
}

// TestALaunchKeyTradesOnce: the fragment stays in browser history, so a spent key must open nothing.
func TestALaunchKeyTradesOnce(t *testing.T) {
	ts, srv := startServerWith(t)
	key := srv.MintLaunchKey()
	trade := func(k string) int {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", nil)
		req.Header.Set(headerLaunch, k)
		res, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := trade(key); code != http.StatusOK {
		t.Fatalf("first trade = %d, want 200", code)
	}
	if code := trade(key); code != http.StatusForbidden {
		t.Fatalf("second trade of the same key = %d, want 403", code)
	}
	if code := trade("never-minted"); code != http.StatusForbidden {
		t.Fatalf("trade of an unminted key = %d, want 403", code)
	}
}

// TestAWindowlessHandoffGetsAKeyForThisInstance: "open Nib again" is how a tab without the token gets
// in, so a launch with no document must be given a window onto the RUNNING instance — it used to be a
// 400, which starts a second Nib and strands the user's documents in the first — and the key it is
// given must actually trade.
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
	res, err := (&http.Client{}).Do(req)
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
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("an expired key traded: %d, want 403", res.StatusCode)
	}
}

// TestAHolderMintsAKeyForAnotherWindow: `POST /api/launch/key` — tier 3's door for its headless
// browser (the P08 phase-close review, R3-7: it shipped with no Go test) — mints a key that trades,
// and refuses a stranger.
func TestAHolderMintsAKeyForAnotherWindow(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)
	res, err := c.Post(ts.URL+"/api/launch/key", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var kr launchKeyResponse
	err = json.NewDecoder(res.Body).Decode(&kr)
	res.Body.Close()
	if err != nil || kr.Key == "" {
		t.Fatal("no key minted")
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", nil)
	req.Header.Set(headerLaunch, kr.Key)
	res2, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("the minted key does not trade: %d", res2.StatusCode)
	}
	if code, _ := stranger(t, http.MethodPost, ts.URL+"/api/launch/key", nil); code != http.StatusForbidden {
		t.Fatalf("a stranger minted a key: %d", code)
	}
}

// TestManyHandoffsDoNotEvictThePrimaryWindowsKey: a file manager starting one process per file hands
// off once per file; eight of those before the primary window traded evicted its key (R3-6).
func TestManyHandoffsDoNotEvictThePrimaryWindowsKey(t *testing.T) {
	ts, srv := startServerWith(t)
	srv.SetHandoffSecret("secret")
	primary := srv.MintLaunchKey()
	c := newClient(t)
	for i := 0; i < 12; i++ {
		handoffTo(t, ts, c, "secret", "")
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", nil)
	req.Header.Set(headerLaunch, primary)
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("after 12 hand-offs the primary window's key no longer trades: %d", res.StatusCode)
	}
}
