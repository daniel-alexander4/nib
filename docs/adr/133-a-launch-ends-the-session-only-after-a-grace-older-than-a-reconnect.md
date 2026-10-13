# ADR-133 — a launch ends the previous session only after a grace older than two reconnects

**Status:** accepted
**Date:** 2026-10-12
**Amends:** ADR-085 decision 1 only. ADR-086 is untouched.
**Context:** /pending 847. ADR-085 decision 1 rested on "a grace runs only after the last window went". A grace
also runs for the length of one stream reconnect (`sseRetry`, 1 s) and of a page reload — measured over 20 reloads
at 95–312 ms, median 203 ms — and a launch takes 17–34 ms from exec to exit, so a launch fits inside either gap.
`keepAliveForHandoff` read any running grace as "the user closed Nib" and `endSessionForLaunch` closed every
document, unsaved edits included, under a window still on screen; the window then reconnected holding stale tabs.
**Applies:** `internal/server/window.go`, `handoff.go`.

## Decision

`keepAliveForHandoff` reports "after the last window" only for a grace at least `launchGapBound` old. A younger
one is cancelled, its documents are kept, and the launch is logged. The bound is `launchGapReconnects` (2) ×
`sseRetry`, a package variable; a test holds it above one reconnect and below `idleExitGrace`.

## Consequences

- Closing Nib and launching it again inside the bound (2 s) keeps the previous documents: extra tabs, never lost
  edits. That is the trade, and it is the cheap side of it.
- A launch landing in the gap still opens a second window — no window is counted at that instant (/pending 848).
- Refused: a per-tab identity carried by the window, with the close deferred until a known id returns. It removes
  the guess, at the cost of a new push and a returning-window protocol; taken up only if the 2 s residue bites.
- Still unmeasured: how often a loopback stream drops in the field.
