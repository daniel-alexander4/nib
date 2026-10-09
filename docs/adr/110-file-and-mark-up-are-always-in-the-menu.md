# ADR-110 — File and Mark Up are always in the menu

**Status:** accepted
**Date:** 2026-10-09
**Context:** Dan: *"File and Mark Up should always be present and not subject to toggling."* ADR-036 let any
main-menu tab but Settings be switched off. `internal/server/settings.go` (`hideableModes`, `alwaysShownModes`,
`shownHiddenModes`), `internal/server/auth.go` (the status), `web/app.js` (`applyModeVisibility`), the Main menu
section of `#settingsFeaturesPage`.
**Supersedes:** ADR-036's *"any main-menu tab can be switched off"* and its *"Settings is the one exemption"* —
there are three. Everything else in it stands.
**Applies:** the main-menu visibility switch.

## Decision

**1. Three tabs cannot be hidden: File, Mark Up and Settings.** The Main menu section has four boxes — Page
Functions, Accessibility, Secure, Signing — and its lead says the other three are always there.

**2. The server refuses them.** `hideableModes` no longer lists `file` or `markup`, so a request naming either
is a 400, as one naming `settings` always was. `alwaysShownModes` names the three; with `hideableModes` it is
every tab in the markup, held by `TestEveryHideableModeIsARealTab`.

**3. A vault that already hides one stops hiding it, with no migration.** The stored list is read through
`shownHiddenModes` where the status is built, which drops what this build may not hide; the window drops any id
that has no box (`applyModeVisibility` reads the boxes, as it reads the mode set, from the DOM). The stored list
is rewritten the next time the user changes a box, because the window sends the whole set from the boxes. Both
halves, deliberately: a tab hidden with no box to bring it back is the one state this switch must never produce.

## Consequences

- A user who had hidden File or Mark Up finds it back after updating, with no message saying so.
- **Covered by** `internal/server/modevisibility_test.go` (the 400s; the stored list filtered),
  `test/jsdom/alwaysshown.test.mjs` (a window booted on a list naming all three hides none, in either list, and
  its next save omits them) and `test/jsdom/modevisibility.test.mjs` (no box for the three).
