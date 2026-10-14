# ADR-169 — Quit closes every window, and a window that cannot close says Nib has quit

**Status:** accepted
**Date:** 2026-10-13
**Supersedes:** the sentence "the window is left to the browser: a process that has gone takes its own window
with it" (`quitNib`'s comment, from PLAN-window-lifetime P01.S06). Everything else of D5/D6 stands: the route
only signals, and the teardown is `run()`'s.
**Context:** Dan, 2026-10-13: "when I hit the io power button, it should kill the process just as it does now
and should also kill the window." Quit posted `/api/quit`, the process exited, and the window stayed: its stream
dropped, `checkSession` found nothing answering, and the window showed "Could not reach Nib — it may have
stopped." The window is the user's own browser's — Nib starts it with `--app=` in the user's profile
(`internal/browser`), where the launched command hands the address to the browser already running and exits —
so there is no process of Nib's to end, and nothing on the server's side can close it.

A page can close itself, within the browser's rule. Measured on the real page against a private nib:
Chrome, address on the command line, app window and plain tab: `history.length` 1, `window.close()` **closed
it**. Chrome through the tier-3 harness (opened blank, then navigated): length 2, **refused**. Firefox 157:
length 1, closed. The page only ever replaces its history entry (`history.replaceState`, two sites), so a
window Nib launched stays at one.
**Applies:** `web/app.js` (`quitNib`, `leaveForQuit`, `closeWouldLose`, `showLaunchOverlay`, the window stream),
`internal/server/window.go` (`handleQuit`, `quitTell`, `handleWindow`), `web/index.html` (the button's title).

## Decision

**1. Every open window is told, and each closes itself.** `handleQuit` wakes the window streams; each writes
`event: quit` and the stream ends. The page's one routine, `leaveForQuit`, marks the page as quitting, puts up
the launch overlay saying "Nib has quit — You quit Nib. You can close this window.", and calls
`window.close()`. The screen goes up FIRST: where the browser closes the window nobody sees it, and where it
refuses the window is left saying what happened instead of an error.

**2. The quit waits for the windows to be told, for at most 500 ms** (`quitTellWait`), and then signals the
exit exactly as before. The exit closes the streams, and a frame written while the server goes down may never
be read; a window that has stopped reading must not be able to hold a quit.

**3. The window Quit was pressed in also leaves when its request is answered** — 2xx, or nothing answering at
all (the process has gone). A refusal closes nothing: Nib did not quit.

**4. Once quitting, the close prompt stands down and the screen cannot be replaced.** `quitting` is read by
`closeWouldLose` — the one door `beforeunload` turns on — because Quit's own dialog has already named what
would be lost and been answered, and the browser's unexplained prompt would otherwise follow it; and by
`showLaunchOverlay`, because the stream dropping next is what used to put "could not reach Nib" up.

## Refused

- **Ending the browser from the server.** The window is a window of the user's browser, shared with whatever
  else they have open in it.
- **Closing only the window that was pressed.** The process ends under all of them.
- **A timer that checks whether the close worked before showing the screen.** The screen is harmless if the
  window goes and needed if it does not.

## Consequences

- A window opened on some other history — a tab used for something else and then pointed at Nib — is left
  open, saying Nib has quit.
- A window that misses the event (the bounded wait ran out, or the write lost to the teardown) sees a dropped
  stream and says it could not reach Nib, as every window did before. It cannot be told from a crash, by design.
- Only the button does this. A signal sent to the process, and the idle exit (which fires when no window is
  left), tell no window.
- Quit's dialog still names only the pressing window's unsaved documents; another window's go with the
  process, as they always did.
- `test/ui/quit.test.mjs` runs its own nib (Quit ends the process) and its own Chrome with the address on the
  command line (the harness's page is one Chrome will not let close). Not measured: a visible app window on a
  desktop — the measurements are headless.
