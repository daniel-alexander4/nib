# ADR-054 — the token is the only credential, and every route but three needs it

**Status:** accepted. **Supersedes ADR-053** in two places — its session cookie, and its list of
routes that "stay public" — and keeps the rest: the single-use launch key in the URL fragment, traded
once, never served by `/api/status`; the hand-off returning a key; the headless log line.

## Context

`/pending 704`, filed by the P08 phase-close review of ADR-053 the same day it landed:

- **ADR-053 left the pre-unlock routes answering anyone.** Its premise was that the wizard routes had
  no token to check — but ADR-053 itself minted the token in `New`, at process start, independent of
  the vault, and the page trades its key before its first `/api/status`. So the premise was false
  and the routes had no remaining reason to be open. Measured: `POST /api/quit` ended Nib for a bare
  `curl` from any user (R3-1); `POST /api/ssh/repoint`, which takes a caller-chosen key path, was an
  unauthenticated, unthrottled **passphrase oracle** for any passphrase-protected SSH key the user
  can read, and a file-existence oracle over the user's filesystem (R3-2); `/api/update/check` let
  any process spend the user's GitHub rate limit; `/api/window` pushed the armed ceremony's `what`.
- **The cookie leaked across ports** (R3-5). Cookies are not isolated by port, so the `HttpOnly`
  `SameSite=Strict` cookie on 127.0.0.1 was sent on every same-site request to any OTHER loopback
  server the user's browser visited — a link to an attacker's listener was enough, since the
  attacker's page's own sub-requests are same-site — and `GET /api/launch` turned the cookie alone
  into the token. The cookie was not the decision ADR-053 recorded from Dan (the fragment key and
  "a tab reopened without it must re-launch nib"); it was added to cover the GETs.
- **A stale token did not read as a stale session** (R7-1): with a cookie, an old tab's reads
  passed and its writes failed "bad csrf token", so the app looked alive while every action failed.

## Decision

1. **The per-process token is the only credential.** No cookie is set. It rides in `X-CSRF-Token` on
   any method, or — on a GET only — in the `auth` query parameter, because `<img src>`, pdf.js and
   `EventSource` cannot set a header. A non-GET is header-only. **The query form grants what a GET
   does, and some GETs act** — `/api/status` runs first-run setup, `/api/window` registers a window,
   the update check calls out, the network test announces — so a URL carrying the token is as good as
   the token for those. Downloads are fetched with the header and saved from a blob, never navigated
   to, so no token-bearing URL enters the address bar or the history.
2. **Every route requires it** through `requireSession`, except the three that carry their own
   secret — `POST /api/launch` (the launch key), `GET /api/instance` (the probe token) and
   `POST /api/handoff` (ADR-006's secret) — and the static files. The wizard, `/api/status`, the
   window stream, quit, the update check and the ceremonies listing included: the page holds the
   token before it calls anything, locked or not. `TestEveryRouteIsBehindTheSessionOrNamed` reads the
   route table from `server.go` and drives every route as a stranger.
3. **The page keeps the token in `sessionStorage`**, which is per origin — scheme, host AND port — so
   a reload of the tab keeps it and no other loopback server can read it. A new tab has none and must
   open Nib again, which is ADR-053's recorded cost. `GET /api/launch` (cookie → token) is gone.
4. **A stale token is refused the same way everywhere**, "no session", so the page shows *Open Nib
   again* on the first call — with words that say the connection was lost and unsaved changes may be
   gone, not that the documents are safe — makes everything behind it inert, and suppresses toasts.
5. **Harnesses** read the keyed log line through one file, `build/launchkey.sh`; "up" means "answers
   HTTP at all", since `/api/status` answers an unauthenticated caller 403.

## Consequences

- Other OS users can neither read nor drive nor stop Nib, nor probe the user's key passphrase,
  through the loopback port. The residuals stand as ADR-053 stated them: a same-user process, and the
  launch key in the browser's argv from launch to trade (`/pending 701`).
- A token in a GET URL appears only in sub-resource requests (images, pdf.js, the window stream) —
  visible to DevTools and to the page itself, both the user's own, and never to the address bar or
  history. It is never sent to another origin: those URLs are same-origin, and the page's Referer to
  other origins is the page URL, which carries no token.
- **ADR-006's secret now grants a window** (a fresh launch key), as ADR-053 already recorded; its
  status line points here.

## Verification

`internal/server/launch_test.go`: the route census; the header, the query form on a GET, a wrong query
token and the query form on a WRITE; no `Set-Cookie`; a key trades once; a windowless hand-off's key
trades; an untraded key expires; `POST /api/launch/key` mints for a holder and refuses a stranger;
twelve hand-offs do not evict the primary window's key. `test/jsdom/launchkey|launchnone|launchlost`:
the fragment is gone before the trade is sent, the token is stored and sent on every request including
the window stream's URL, a status answer's token is never used, a reload trades nothing, a non-session
403 leaves the screen down, and the screen makes the page inert.
