# PLAN — the window owns the process's life

**Dateline.** Seeded 2026-09-06 from `/grill` on the orphan-process defect, whose measurements are
this plan's factual base. Dan settled the product call: **closing the last window exits Nib**, with a
notice first when a ceremony is still running.

**Where this plan and the brief differ, the plan wins.** The mechanism here is *not* the one first
proposed to Dan — a heartbeat was overturned during the grill and the reason is recorded in D1,
because it is the kind of thing a later reader will otherwise "simplify" back.

**Status: unbuilt.** Tracked as `/pending 375`.

---

## The defect, reproduced

In a throwaway `HOME` on 2026-09-06, with the real binary:

- The window was killed. **`nib` kept running**, kept answering **HTTP 200**, kept its loopback port,
  and kept owning `instance.json`.
- A later bare launch could not hand off — **400, "a path is required"** (`internal/server/handoff.go:65`)
  — and started a **second server alongside** the first, so processes accumulate.
- In real use the vault is unlocked by then, so an orphan holds an **unlocked vault in memory and an
  open port** after the user believes Nib is closed. (Reasoned, not measured — the probe never
  enrolled a key.)

**Why.** `run()` blocks on a `select` over exactly two things — SIGINT/SIGTERM, or a server error
(`cmd/nib/main.go:206-213`). `browser.Open`'s returned command is used only for its error at
`main.go:185`; the reap inside `Open` (`browser.go:102-108`) exists to prevent a zombie and reports
to nobody. Named searches returning **zero**: `heartbeat|keepalive|/api/ping|lastSeen` in Go;
`pagehide|unload|sendBeacon` in `web/app.js`; `Flusher|text/event-stream` in `internal/server`;
`setInterval` in `web/app.js`. Closing a window is invisible to the server, by construction.

**Why the obvious fix is wrong.** The child process is not the window. `Open` runs `chrome --app=…`,
and when a browser is *already running* that invocation hands off the URL and exits at once — which
is why `alive(cmd, appModeSettle)` exists (`browser.go:58,87`). "Exit when the child exits" would
kill Nib a quarter-second after startup for anyone who already had a browser open.

## The law this establishes

**A window's life is a connection, not a timer.** Anything that decides whether a window still exists
reads the liveness of a socket the window holds open. A closed page drops its connection; a minimised
or frozen page does not. No poll, counter or timestamp may be substituted for that, because none of
them can tell those two states apart.

---

## Decisions

### D1 — The signal is a dropped connection, not a heartbeat *(settled 2026-09-06 via /grill)*
The first proposal was a client heartbeat with an idle timeout. It was **overturned**: a background
window has its timers throttled to roughly once a minute and can be frozen outright, so a heartbeat
cannot distinguish a minimised window from a closed one — and that distinction is the entire
feature. Getting it wrong exits Nib under a working user, which is worse than the orphan being
fixed. A long-lived stream inverts it: the socket is held by the page's existence, not by its
ability to run JavaScript on time.

### D2 — Idle-exit is armed only if this process launched a browser *(settled 2026-09-06 via /grill)*
Not a heuristic: every harness runs `NIB_NO_BROWSER=1` (`build/winrepro.sh:151,285,326`,
`build/ceremonyrepro.sh:59,520`, and the uirepro/pairrepro pair), and the `NIB_ADDR` headless mode
exists for SSH-tunnel use. A process that never opened a window is never waiting for one, so the
rule is exact and needs no allow-list to drift.

### D3 — The stream is guarded at the pre-unlock class *(settled 2026-09-06 via /grill)*
`requirePublicLoopback`, as `GET /api/status` and `POST /api/handoff` already are — **not**
`requireUnlocked`. A window sitting on the unlock screen is a real window; behind `requireUnlocked`
it would hold no stream, and Nib would exit while the user was typing a passphrase.

### D4 — The grace timer is cancellable from two directions, and they are two counters *(settled 2026-09-06 via /grill)*
A new window connecting cancels it, and so does an inbound hand-off. The hand-off case is a race the
grill surfaced and neither party had named: close, relaunch immediately, and the file is handed to a
process that is already exiting. They are counted separately because they fail differently.

### D5 — `beforeunload` stops the close; the wording lives in Quit *(settled 2026-09-06 via /grill)*
Browsers deliberately ignore custom `beforeunload` text, so the X-button path can *require
confirmation* but cannot say why. It is therefore armed **only** when something would be lost — an
armed or running ceremony, or unsaved changes — and an explicit **Quit Nib** action carries the real
wording in Nib's own modal. A prompt on every close would train the user to dismiss it.

### D6 — Every exit path runs the same teardown, in the same order *(settled 2026-09-06 via /grill)*
`DisarmSession`, close the server, drop the instance record — the order `run()` already uses. Adding
a third exit cause (`last-window`) beside signal and serve-error must not add a third teardown; this
is ADR-009's shape, and the failure it prevents is the stale record returning by a new door.

### D7 — Windows are counted, never booleaned *(settled 2026-09-06 via /grill)*
"A window is connected" cannot distinguish *one of two closed* from *the last closed*, and the exit
decision turns on exactly that. The observable is a count from the start.

### D8 — Unsaved work uses the same door as the ceremony notice *(settled 2026-09-06 via /grill)*
A dirty document prompts on close through D5's mechanism rather than growing a second one. Today the
orphan silently preserved unsaved work; exiting cleanly must not silently discard it.

---

## Build order

### P01 — Closing the window ends the process
**Goal.** Nib exits when its last window goes away, never while one still exists, and never during a
headless run — with a notice first when a ceremony or unsaved work would be lost.

**Exit criteria.**
- No `nib` process and no `instance.json` survive a closed window; a later launch starts fresh.
- A window minimised past the browser's freeze threshold does **not** exit Nib.
- All three harnesses complete unchanged, with `idleExitArmed` observably false in each.

#### P01.S01 — the stream and the window count *(done 2026-09-07, v1.125.5)*
Scope: one `requirePublicLoopback` stream route a window holds open; the server counts live streams.
No exit behaviour yet. Refs: D1, D3, D7.
Acceptance:
- Opening a window raises the count; closing it lowers it, asserted at tier 3.
- Two windows count two, and closing one leaves one.
- No goroutine or request context outlives its stream, asserted under `-race`.
Tasks: *(written at slice-grill time, 2026-09-07)*
1. T01 — `GET /api/window` under `requirePublicLoopback`: an SSE stream that holds until the
   client disconnects, incrementing a live-window count for its lifetime.
2. T02 — log each transition with a stable literal, because a tier-3 test cannot read a Go
   accessor and the count must not become a published `/api/status` field for a test's benefit.
   The literal is the seam inventory's *Emitted string* and is verified against captured output.
3. T03 — the client opens the stream at boot and never closes it; a second window is a second
   stream and therefore a count of two.
4. T04 — tier-1 tests for the arithmetic: one connection counts one, two count two, a disconnect
   decrements, and nothing outlives its stream under `-race`. **The helper cancels client-side
   before `ts.Close()`** — `httptest`'s Close waits for outstanding requests, so a leaked stream
   hangs the suite rather than failing it.
5. T05 — tier-3: opening the real app logs a connect, closing the window logs a disconnect.
6. T06 — instrument inventory rows P1 and S1 as part of this slice, per `instrument.md`.

**Divergence from the task list, recorded rather than absorbed (2026-09-07).** Two files outside
T01–T06 changed, both because an existing guard correctly refused the slice:
- `test/jsdom/docid.test.mjs` — ADR-004's bypass scan flagged `/api/window` as a document route
  reached without `apiFetch`. It is not one, and it *could not* be pinned regardless: EventSource
  cannot set request headers, so neither `X-Nib-Doc` nor a CSRF token can ride on it. Added to the
  allow-list with that reason, as the scan's own comment requires.
- `build/uirepro.sh` — the tier-3 file-count floor moved 22 → 23. Its own comment records this guard
  going stale for eight versions once before, so moving it is part of adding a file, not a chore.

#### P01.S02 — measure the assumption before building on it
Scope: **a measurement, not code.** Does an app-mode window minimised past the browser's freeze
threshold keep its stream open? Refs: D1.
Acceptance:
- A recorded observation spanning the 5-minute threshold, not a reasoned one.
- **If the socket does not survive, D1 is superseded in place and the rest of P01 is re-planned** —
  this slice is a gate, and it is second on purpose.

#### P01.S03 — arm the idle-exit, and prove it stays disarmed
Scope: arm only when this process launched a browser; log `idleExitArmed` once at startup.
Refs: D2.
Acceptance:
- Every harness reports it false, asserted rather than observed by eye.
- A normal launch reports it true.
- A red proof: arming unconditionally turns a harness red.

#### P01.S04 — the grace timer and its two cancels
Scope: last stream closes → grace → exit; cancelled by a new window or an inbound hand-off, counted
per cause. Refs: D4, D6, D7.
Acceptance:
- A reload does not exit Nib; the reconnect is measured against the grace rather than assumed.
- A hand-off during grace cancels the exit and the file opens.
- Exit runs D6's teardown for the new `last-window` cause exactly as for a signal.

#### P01.S05 — the close prompt
Scope: `beforeunload` armed only for an armed/running ceremony or unsaved changes. Refs: D5, D8.
Acceptance:
- Closing with nothing to lose does not prompt.
- Closing with a ceremony armed prompts, and confirming disarms through the existing path.
- The armed/unarmed decision has one door, not one per condition.

#### P01.S06 — Quit Nib
Scope: an explicit quit action whose modal names what it will end, in Nib's words. Refs: D5, D6.
Acceptance:
- The modal names a live ceremony and an unsaved document specifically, not generically.
- Quit exits through the same teardown as every other cause.

---

## Out of scope

- **A tray presence.** It was considered and dropped when the product call became "exit on close":
  nothing outlives the window, so there is nothing to make visible. It would also have put a cgo
  dependency on macOS in a tree whose whole build posture is `CGO_ENABLED=0`.
- **Keeping a ceremony alive past the last window.** Explicitly decided against; the notice exists
  so the user knows that is what closing means.
- **Any change to how a browser is launched or discovered.** `internal/browser` is untouched.

## Standing caveats

- **The plan's load-bearing assumption is measured in S02, not asserted here.** If a minimised window
  drops its socket, D1 is wrong and the design changes; that is why the measurement is a gate rather
  than a verification step at the end.
- **S02's answer has no standing guard afterwards.** No tier can hold a window minimised for five
  minutes, so this is measured once and recorded, and a future browser change could invalidate it
  silently.

## Seam inventory

`~/.claude/projects/-home-dan-repos-nib/memory/instruments/window-lifetime.md` — 19 rows (7 paths,
6 seams, 6 gap-downs), written against this plan before any code. **No hot-path rows**, recorded
explicitly; S1 is flagged as a goroutine-lifetime row, and S6 is the only
`diagnostic, no standing reader` entry.
