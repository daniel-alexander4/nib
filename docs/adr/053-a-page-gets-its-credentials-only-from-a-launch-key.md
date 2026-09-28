# ADR-053 — a page gets its credentials only from a launch key

**Status:** accepted. **Supersedes ADR-006's premise** — *"nib's CSRF token lives only in memory, so no
co-resident process can drive its API"* — which was false when it was written. ADR-006's decision (a
separate, single-purpose hand-off credential on disk) stands, amended by one grant below.

## Context

`/pending 685`, found by the P07 phase-close review. `GET /api/status` is public so the first-run wizard
can run, and is guarded by `requirePublicLoopback`, i.e. `originIsLoopback`. That check passes a request
carrying neither `Sec-Fetch-Site` nor `Origin` — which is what curl, or any program that is not a browser,
sends. `/api/status` put the per-process CSRF token in its body once the vault was unlocked. So:

- **Every process on the machine could read the token and drive every write** — `/api/write` (bytes to
  any path, as the user), vault import, the signing routes. TCP loopback is not per-user, so that included
  processes belonging to **other OS users** on a shared machine, who cannot otherwise touch this user's
  files.
- **The reads needed no token at all.** `requireUnlocked` checked the token on non-GET methods only, so
  `/api/pdf`, `/api/doc`, `/api/listdir`, `/api/identity` answered any local caller. The entry as filed
  named only the token; the reads were found tracing it.

No header-based fix exists. `Sec-Fetch-Site` and `Origin` are forbidden header names only to browser
script; a program sets them to whatever it likes. Binding a Unix socket would remove the browser UI.
So the fix is a change to where the page's secret comes from, which is an auth-model decision; Dan chose
it on 2026-09-28 (option A of the item's grill).

## Decision

1. **A single-use launch key travels in the URL FRAGMENT Nib opens its window at**, `http://127.0.0.1:P/#k=…`.
   A fragment is never sent to a server, never logged by one, and never carried in a Referer. `cmd/nib`
   mints one per window (`Server.MintLaunchKey`); the page removes it from the address bar before doing
   anything else and trades it once at `POST /api/launch`. A traded key is retired, so a URL left in
   browser history opens nothing, and an untraded one expires after ten minutes (`launchKeyTTL`) — a
   key minted for a window that never opened does not stay good for the life of the process.
2. **The trade returns two per-process credentials**: an `HttpOnly`, `SameSite=Strict` session cookie,
   named per port (`nib_<port>`, since cookies are not isolated by port and two Nibs may run side by
   side), and the CSRF token. **The cookie authenticates every request, GETs included** — it is the only
   credential a sub-resource load can carry (`<img src>`, pdf.js, a download navigation). The token is
   still sent as `X-CSRF-Token` on writes, and a request carrying it is authenticated too; that is how the
   headless harnesses and the Go tests reach the API.
3. **One door**: `requireSession` (session on every method; token and loopback origin on writes).
   `requireUnlocked` is `requireSession` plus an open vault, and checks the session FIRST, so an
   unauthenticated caller is answered the same 403 whatever the vault's state.
4. **`/api/status` carries no token**, nor does any response a caller can obtain without a key or the
   session. The token and cookie are minted in `New`, per process, independent of the vault: a page
   keeps them across a lock, an unlock and a vault import (which used to rotate the token).
5. **A reload or a second tab in the same browser session** recovers the token from `GET /api/launch`
   with its cookie. A page with neither key nor cookie shows *Open Nib again* and does nothing else, and
   so does a page whose session dies under it (`apiFetch` on a 403 `no session` — a restart on a pinned
   `NIB_ADDR` port leaves the page holding a cookie the new process never issued).
6. **A second launch receives a fresh key through the hand-off.** `POST /api/handoff` returns `launch` on
   every result, and a launch with no document now answers `window` rather than 400 — which used to send
   the launch down its become-primary path, and would now strand the user's open documents in the first
   instance behind a window that cannot reach it. **This is the one grant ADR-006's secret gains** beyond
   its verb.
7. **Headless**: when no window was launched (`NIB_NO_BROWSER`, or the browser failed to start) the URL
   with its key is logged, for the person who started the process. That log line is how tiers 3, 4, 6 and
   the Windows harness get in. A key is never logged when a window was opened.

## Consequences

- **Other OS users can no longer read or drive Nib through the token, and no longer at any time they
  choose**: they hold neither the cookie nor the hand-off secret (a `0600` file in the user's config
  directory), and `/api/status` gives them nothing.
- **Except in one window, declared and filed (`/pending 701`): the key is in the browser's ARGV** from
  launch until the page trades it. `browser.Open` passes `--app=<url>`, and on Linux without `hidepid`
  `/proc/<pid>/cmdline` is world-readable (measured on the development machine: `/proc` mounted without
  it). A process of another user polling the process table can trade the key first; the real window then
  shows *Open Nib again*, and the thief holds a session. The window is launch-to-trade, about a second.
  Closing it means the URL never appears in argv — a `0600` trampoline file the browser is pointed at —
  which must be verified against snap and flatpak confinement, macOS and Windows before it ships,
  because a trampoline a confined browser cannot read is a Nib that does not open. That verification is
  not possible on the machine this was built on, so it is not guessed at here.
- **The residual, stated plainly: a process running as the same user can still get in** — it can read
  the hand-off secret, the browser's cookie store, or the log of a headless run. It already has the
  user's files and can ptrace the user's processes; Nib cannot defend a boundary the OS does not draw.
  ADR-006's own "stronger alternative" (kernel-vouched peer credentials) is still the only thing that
  would narrow this, and still refused for the reason that ADR gives.
- **The cookie is sent to every port on 127.0.0.1.** Another local server receives it if the browser
  navigates there from a same-site page (SameSite=Strict withholds it on navigations from other sites).
  Nib never links there; a link inside a PDF the user opens and clicks could. Declared, not closed.
- **Public routes stay public**: `/api/status`, `/api/ceremonies`, `/api/ceremony/next`, `/api/window`
  and the pre-unlock wizard routes answer without a session, as the locked panel and first run need.
  The ceremonies listing is readable by any local process — a residue this ADR does not change.
- **A tab opened by hand in a browser that never traded a key must open Nib again** — the stated cost of
  the decision.
- **Across an update boundary** a pre-053 launcher handing off to a post-053 instance opens a URL with
  no key, and the window shows *Open Nib again*; relaunching with the current binary works. Transient.
- **`GET /api/window` stays public** (a window on the unlock screen is a real window, D3), so a local
  process can still subscribe to it and read what it pushes — the armed ceremony's `what` and download
  progress. Pre-existing and unchanged here; filed as `/pending 702`.

## Verification

`internal/server/launch_test.go` pins five claims — status never carries the token (read raw, by JSON
key), a caller without the session is refused a GET and a write while the token still authenticates, a
key trades once and its cookie authenticates, and a windowless hand-off returns a key that trades, an untraded key expires — each
probed red by mutation. `test/jsdom/launchkey.test.mjs` and `launchnone.test.mjs` pin the page: the key is
sent in `X-Nib-Launch`, removed from the address bar, and its token is the one writes carry; with no
session the page shows *Open Nib again* and never boots; `launchlost.test.mjs`, that a 403 `no session`
mid-run puts the same screen up.
