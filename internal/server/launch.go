package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"time"
)

// How a page comes to hold Nib's credentials (ADR-053, `/pending 685`).
//
// **The CSRF token used to be served by `GET /api/status` to any loopback caller**, and
// `originIsLoopback` passes a request carrying neither `Sec-Fetch-Site` nor `Origin` — curl, or any
// program. So every process on the machine, including one belonging to ANOTHER OS user (TCP
// loopback is not per-user), could read the token and drive every `requireUnlocked` route. And the
// GETs needed no token at all: `requireUnlocked` checked it on writes only, so `/api/pdf`,
// `/api/doc` and `/api/listdir` answered anyone. No header-based fix exists — curl sets
// `Sec-Fetch-Site` and `Origin` freely; they are forbidden headers only to browser script.
//
// **So the secret now travels where only the launching user can see it**: in the URL FRAGMENT Nib
// opens its window at (`/#k=<launch key>`). A fragment is never sent to a server, never logged by
// one and never carried in a Referer. The page trades the key ONCE (`POST /api/launch`) for two
// per-process credentials:
//
//   - a session cookie — HttpOnly, SameSite=Strict — which authenticates EVERY request, GETs
//     included, and is the only credential a sub-resource load (`<img src>`, pdf.js, a download
//     navigation) can carry; and
//   - the CSRF token, which the page sends as `X-CSRF-Token` on writes, as it always has.
//
// A reload or a second tab in the same browser session carries the cookie and recovers the token
// from `GET /api/launch`; a tab in a browser that never traded a key has neither and must open Nib
// again. A second launch receives a fresh key through the hand-off (ADR-006's credential), so
// "open Nib again" reaches the running instance rather than starting a second one.

// maxLaunchKeys bounds the outstanding (minted, untraded) keys. A key is minted per window Nib
// opens, so a handful is the real population; the bound exists so a launch loop cannot grow the
// list without limit. The OLDEST is dropped, because the newest is the window just opened.
const maxLaunchKeys = 8

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

// sessionCookieName is per PORT. Cookies are not isolated by port, so two Nibs running side by
// side on 127.0.0.1 would otherwise overwrite each other's cookie and each lock the other's page
// out. The Host guard has already confined `r.Host` to loopback.
func sessionCookieName(r *http.Request) string {
	_, port, err := net.SplitHostPort(r.Host)
	if err != nil || port == "" {
		port = "80"
	}
	return "nib_" + port
}

// authenticated reports whether the request carries this process's session cookie or its CSRF
// token. Either is proof the caller got its credentials through a launch key: neither is served to
// anyone who does not already hold one.
func (s *Server) authenticated(r *http.Request) bool {
	s.mu.Lock()
	session, csrf := s.session, s.csrf
	s.mu.Unlock()
	if c, err := r.Cookie(sessionCookieName(r)); err == nil &&
		subtle.ConstantTimeCompare([]byte(c.Value), []byte(session)) == 1 {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) == 1
}

// requireSession is the ONE door every credentialled route passes (ADR-009): the caller must hold
// the session, and a write must also carry the CSRF token and a loopback origin. `requireUnlocked`
// is this plus an open vault.
//
// **Every method, GETs included** — the defect this closes was as much the reads as the token.
func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticated(r) {
			httpError(w, http.StatusForbidden, "no session")
			return
		}
		if r.Method != http.MethodGet {
			s.mu.Lock()
			csrf := s.csrf
			s.mu.Unlock()
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) != 1 {
				httpError(w, http.StatusForbidden, "bad csrf token")
				return
			}
			if !originIsLoopback(r) {
				httpError(w, http.StatusForbidden, "bad origin")
				return
			}
		}
		next(w, r)
	}
}

type launchResponse struct {
	CSRF string `json:"csrf"`
}

// handleLaunchTrade is `POST /api/launch`: a launch key in, the session cookie and the CSRF token
// out. The key is retired on success, so a fragment left in browser history opens nothing.
func (s *Server) handleLaunchTrade(w http.ResponseWriter, r *http.Request) {
	if !s.consumeLaunchKey(r.Header.Get(headerLaunch)) {
		httpError(w, http.StatusForbidden, "unknown or used launch key")
		return
	}
	s.mu.Lock()
	session, csrf := s.session, s.csrf
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName(r),
		Value:    session,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, launchResponse{CSRF: csrf})
}

// handleLaunchResume is `GET /api/launch`: a page that already holds the session (a reload, a
// second tab) recovers the CSRF token, which lives only in page memory.
func (s *Server) handleLaunchResume(w http.ResponseWriter, r *http.Request) {
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
