package server

import (
	"crypto/subtle"
	"net/http"
	"time"
)

// How a page comes to hold Nib's credentials (ADR-053, `/pending 685`; ADR-054, `/pending 704`).
//
// **The CSRF token used to be served by `GET /api/status` to any loopback caller**, and
// `originIsLoopback` passes a request carrying neither `Sec-Fetch-Site` nor `Origin` — curl, or any
// program. So every process on the machine, including one belonging to ANOTHER OS user (TCP
// loopback is not per-user), could read the token and drive every `requireUnlocked` route. No
// header-based fix exists — curl sets `Sec-Fetch-Site` and `Origin` freely.
//
// **So the secret travels where only the launching user can see it**: in the URL FRAGMENT Nib opens
// its window at (`/#k=<launch key>`), never sent to a server, logged or carried in a Referer. The
// page trades the key ONCE (`POST /api/launch`) for the per-process token, and the token is the only
// credential there is:
//
//   - **every route but three** requires it (`requireSession`) — `POST /api/launch`,
//     `GET /api/instance` and `POST /api/handoff` each carry their own secret, and the static files
//     are the page itself;
//   - on any method it rides in the `X-CSRF-Token` header; on a GET it may instead ride in the
//     `auth` query parameter, because `<img src>`, pdf.js and `EventSource` cannot set a header. A
//     non-GET is header-only. The query form grants whatever a GET does — and some GETs ACT
//     (`/api/status`'s first-run setup, a window registering on `/api/window`, the update check, the
//     network test) — so a URL carrying the token is as good as the token for those; it is the
//     user's own, and the page never puts one where another origin or the history can keep it.
//
// **There is no cookie** (ADR-054 supersedes ADR-053 here). Cookies are not isolated by port, so a
// session cookie on 127.0.0.1 was sent to every other loopback server the user's browser visited —
// a link to an attacker's listener was enough — and the cookie alone recovered the token. The page
// keeps the token in `sessionStorage`, which IS isolated by port (it is per origin), so a reload
// keeps it and a new tab does not: a tab that never traded a key must open Nib again, as ADR-053's
// decision stated. A second launch receives a fresh key through the hand-off (ADR-006's credential),
// so "open Nib again" reaches the running instance rather than starting a second one.

// maxLaunchKeys bounds the outstanding (minted, untraded) keys. A key is minted per window Nib
// opens, so a handful is the real population; the bound exists so a launch loop cannot grow the
// list without limit. The OLDEST is dropped, because the newest is the window just opened.
//
// 32, not 8 (the P08 phase-close review, R3-6): a file manager that starts one process per file hands
// off once per file, and eight of those before the primary window traded evicted the primary's key.
const maxLaunchKeys = 32

// launchKeyTTL is how long an untraded key stays good. A window trades within seconds; the margin
// is for a person opening a headless run's logged URL by hand, over an SSH tunnel. Without it a key
// minted for a window that never opened — the browser failed, or a hand-off's window was never
// drawn — stayed valid for the life of the process.
const launchKeyTTL = 10 * time.Minute

// headerLaunch carries a launch key to `POST /api/launch`. A header rather than a body so the key
// never passes through a JSON decoder or a log of request bodies.
const headerLaunch = "X-Nib-Launch"

// launchKey is one outstanding key and when it was minted.
type launchKey struct {
	key    string
	minted time.Time
}

// MintLaunchKey issues a single-use key for the URL fragment of a window about to be opened.
func (s *Server) MintLaunchKey() string {
	k := newToken()
	s.mu.Lock()
	s.launchKeys = append(s.launchKeys, launchKey{key: k, minted: time.Now()})
	if len(s.launchKeys) > maxLaunchKeys {
		s.launchKeys = append([]launchKey(nil), s.launchKeys[len(s.launchKeys)-maxLaunchKeys:]...)
	}
	s.mu.Unlock()
	return k
}

// consumeLaunchKey reports whether k was an outstanding, unexpired key, and retires it if it matched
// (an expired match is retired too, and refused).
// Compared in constant time against every candidate, so the scan's timing says nothing about
// which prefix matched.
func (s *Server) consumeLaunchKey(k string) bool {
	if k == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hit := -1
	for i, c := range s.launchKeys {
		if subtle.ConstantTimeCompare([]byte(k), []byte(c.key)) == 1 {
			hit = i
		}
	}
	if hit < 0 {
		return false
	}
	fresh := time.Since(s.launchKeys[hit].minted) < launchKeyTTL
	s.launchKeys = append(s.launchKeys[:hit:hit], s.launchKeys[hit+1:]...)
	return fresh
}

// queryAuth names the query parameter a GET may carry the token in. `t` is taken (cache-busters).
const queryAuth = "auth"

// authenticated reports whether the request carries this process's token — in the header on any
// method, or in the `auth` query parameter on a GET. The token is served only to a caller that traded
// a launch key.
func (s *Server) authenticated(r *http.Request) bool {
	s.mu.Lock()
	csrf := s.csrf
	s.mu.Unlock()
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) == 1 {
		return true
	}
	return r.Method == http.MethodGet &&
		subtle.ConstantTimeCompare([]byte(r.URL.Query().Get(queryAuth)), []byte(csrf)) == 1
}

// requireSession is the ONE door every credentialled route passes (ADR-009): the caller must hold
// the token (header on any method, `auth` query on a GET), and a write must also carry a loopback
// origin. `requireUnlocked` is this plus an open vault. `TestEveryRouteIsBehindTheSessionOrNamed`
// holds every registered route to it.
//
// **Every method, GETs included** — the defect this closes was as much the reads as the token.
func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticated(r) {
			httpError(w, http.StatusForbidden, "no session")
			return
		}
		// A write authenticated through the header (`authenticated` accepts the query form on GETs
		// only), so what is left for a write is the origin.
		if r.Method != http.MethodGet && !originIsLoopback(r) {
			httpError(w, http.StatusForbidden, "bad origin")
			return
		}
		next(w, r)
	}
}

type launchResponse struct {
	CSRF string `json:"csrf"`
}

// handleLaunchTrade is `POST /api/launch`: a launch key in, the token out. The key is retired on
// success, so a fragment left in browser history opens nothing.
func (s *Server) handleLaunchTrade(w http.ResponseWriter, r *http.Request) {
	if !s.consumeLaunchKey(r.Header.Get(headerLaunch)) {
		httpError(w, http.StatusForbidden, "unknown or used launch key")
		return
	}
	s.mu.Lock()
	csrf := s.csrf
	s.mu.Unlock()
	writeJSON(w, launchResponse{CSRF: csrf})
}

type launchKeyResponse struct {
	Key string `json:"key"`
}

// handleLaunchKey is `POST /api/launch/key`: a caller already holding the session mints a key for
// another window. It grants nothing the caller does not have; tier 3 uses it to put its headless
// browser through the same door a real window takes.
func (s *Server) handleLaunchKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, launchKeyResponse{Key: s.MintLaunchKey()})
}
