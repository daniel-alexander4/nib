# ADR-085 — a launch into a windowless Nib is a new session, and what was open is remembered by path

**Status:** accepted (2026-10-05). Extends PLAN-window-lifetime D4 (the hand-off's cancel) and ADR-006.

## Context

A Nib whose last window has closed lives for `idleExitGrace` (10 s) so that a reload finds what it left. A launch
arriving in that grace is handed to the old process and cancels its exit (D4, `/pending 727`) — and the window it
surfaced then adopted every document the closed window had held. Closing Nib and starting it again a few seconds later
reopened everything; waiting eleven seconds reopened nothing.

Measured on the installed 1.173.2 (X11, Chrome app mode, one run each): the last window closed and the process exited
10.04 s later; a launch 3.7 s after the close logged `idle-exit cancelled by a hand-off` and the process, with its
documents, survived. Nothing else can bring a document back — the vault held only `Recent`, the page stores only its
token, and the bind address is a random port, so a browser-restored tab cannot reach a new process.

## Decision

1. **A hand-off that cancels a running grace ends the previous session.** `keepAliveForHandoff` reports whether a
   grace was running, read under the same hold of `idle.mu` that cancels it; `handleHandoff` then closes every document
   through the existing close-all door (`setDoc(nil)`) before it reads the path it was handed. A grace runs only after
   this process's last window went, so this is exactly "the user closed Nib and started it again".
2. **A reload is not a launch and keeps everything.** It returns as a window on the stream and never passes through
   the hand-off. A launch while a window is open adds a document and closes nothing, as before. A process that never
   armed the idle exit (headless, `NIB_ADDR`) has no grace and is unchanged.
3. **What was open is recorded by path, in the vault** (`Contents.LastOpen`, beside `Recent`), by one door
   (`recordLastOpen`) with two callers: the last window going — before the grace is armed — and Quit, before the exit is
   requested, because the teardown does not wait for a handler's vault write. Best-effort, and logged.
4. **A session that ends with no file open leaves the record alone**, so opening Nib and closing it does not cost the
   user the session before.
5. **The launch state offers it back**: `GET /api/lastopen` and *Resume last session*, which reopens each path through
   the ordinary open door, in tab order.

## Consequences

- Paths only. A document with no file behind it (an upload, a combine, a conversion, an arrival) is not resumable, and
  an unsaved edit is not carried: the file is read from disk. The close prompt already asked about unsaved work; a
  browser that died without asking, followed by a launch inside the grace, now loses it where it used to be kept. That
  case is logged (`previous session ended by a launch: N closed, M with unsaved changes`).
- The record is file paths at rest, in the encrypted vault, with `Recent`'s residue and no more.
- **Not addressed:** a launch while a window is open still surfaces a second window on the same process (ADR-006: no
  reliable raise). A Nib with a window left open somewhere has not been closed, and its documents are still open.
- The refused alternatives: closing the documents when the last window goes (a reload would come back empty), and
  shortening the grace (a narrower race, the same defect).

Guards: `internal/server/lastopen_test.go` (the boundary both ways, the record, Quit), `test/jsdom/pan.test.mjs` and
`test/ui/pan.test.mjs` (the offer, and a real window's stream dropping).
