# ADR-086 — a launch into a running Nib is a tab in the window that is already open

**Status:** accepted (2026-10-06). Supersedes ADR-085's "Not addressed" consequence. Extends ADR-006 and ADR-085.

## Context

A launch hands its document to the running Nib (ADR-006), which installs it. The window that was already open had no
way to hear of it: the window stream carried the armed state and the download and nothing else, and a page asks what
the server holds only at boot and on a 409. So the launch always opened another window, and that window's boot restore
showed every document. To the user this read as a second Nib that had reopened everything; N launches left N windows,
and closing one did not close Nib (`/pending 845`).

Tracing the path found four more ways "a running Nib takes it as a tab; a Nib that is not running opens fresh" broke:

- **Launches that start together made several Nibs.** A file manager opening several documents runs `nib %f` once per
  file. Each finds no instance record; all but one lose the exclusive create — and carried on serving. Measured
  (four launches at once, isolated home, headless, six rounds): 1, 1, 1, 2, 1 and 4 serving processes.
- **A launch behind one whose window had not connected yet opened its own**, since no window was counted.
- **`?open=` stayed in the window's address**, so a reload reopened a document the user had closed.
- **A path queued while locked outlived its session**, and opened at the next launch's unlock.

A page cannot bring its own window forward: in Chrome app mode on X11, `window.focus()` from a timer left the active
window unchanged with the Nib window covered and with it minimised (one run each). `wmctrl -i -a` on the window found by
its class raised it in both states (Muffin, five of five).

## Decision

1. **A hand-off is pushed to the open windows.** The window stream gains a `handoff` event carrying the route's
   outcome and a sequence. The page reconciles with the server — the one function that makes its tabs match — so the
   document comes up as the front tab; a refused or queued launch is said as the notice it used to carry on a URL. A
   window hears the hand-offs that arrive while it is open and never the ones before it connected.
2. **The answer says whether a window has it** (`surfaced`), from one function (`windowHas`), and the launch opens a
   window only when it is false. True when a window is open, or when one has been asked for and has not connected yet:
   by this process's own launch (`ExpectWindow`, before the first request can be served) or by an earlier answer of
   "open one". The wait is `windowExpectedFor`, 30 s — measured against a fresh browser profile at 7.5–10.0 s (three
   runs) — and a window that never comes is logged and forgotten.
3. **Absent means open a window.** The launch and the running Nib can be two builds for the length of an upgrade;
   a build without the field reads and writes what it always did.
4. **A launch with no document always gets a window**, whatever is open: it is how a page that never held the session
   gets in (ADR-053). **A refusal needs a window that is open**, not one on its way, because the refusal is a sentence.
5. **A launch that loses the instance record hands off to the winner** instead of serving beside it. The winner bound
   its listener before it published, so the probe waits on a socket that exists.
6. **A launch's message on the URL is spent once**: the page removes `open` and `notice` after reading them.
7. **A session's end clears its locked queue** (`endSessionForLaunch`).
8. **The raise is asked for where the machine allows it, and never relied on** (`browser.Raise`): X11, by the window
   class Nib already sets, through `wmctrl` or `xdotool` if the user has one. Everywhere else the window takes the tab
   where it is and marks its title until it is next focused.

## Consequences

- One window per Nib for document launches. Closing it closes Nib, and ADR-085's fresh start then applies.
- **Declared gap: a Nib window that is covered or minimised gains the tab without coming forward** on Wayland, macOS,
  Windows, and X11 without a helper. The title mark is what a taskbar shows. ADR-006's finding stands; decision 8 is an
  opportunistic helper and not a platform implementation.
- **Not addressed:** launching Nib with no document while it is running still opens a second window (decision 4).
- A counted window that cannot draw — a frozen or discarded browser tab — is sent the event and shows nothing. Logged
  (`hand-off taken by an open window:`), not detected.
- The refused alternatives: closing the old window and moving everything to a new one (unbaked annotations and form
  values live in the page); a platform raise per OS (ADR-006).

Guards: `internal/server/handoffpush_test.go`, `internal/instance` (`TestAHandOffReplyWithoutSurfacedMeansOpenAWindow`),
`cmd/nib` (`TestSeveralLaunchesAtOnceBecomeOneNib`), `test/jsdom/handoffpush.test.mjs`, and `test/ui/pan.test.mjs`
(the real binary launched against a real window).
