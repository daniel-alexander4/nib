# ADR-112 — Page Functions is always in the menu

**Status:** accepted
**Date:** 2026-10-09
**Context:** Dan, after ADR-110: *"page functions should also always be available to user."* The tab's id is
`edit`. `internal/server/settings.go` (`hideableModes`, `alwaysShownModes`), the Main menu card of
`#settingsFeaturesPage`.
**Supersedes:** ADR-110 §1's count only — four tabs cannot be hidden, not three. Its mechanism is unchanged and
is what this uses.
**Applies:** the main-menu visibility switch.

## Decision

**Four tabs cannot be hidden: File, Mark Up, Page Functions and Settings.** `edit` moves from `hideableModes` to
`alwaysShownModes`; its box is gone, so the Main menu card has three — Accessibility, Secure, Signing. A request
naming `edit` is a 400, and a vault that already hides it stops hiding it by ADR-110 §3's two halves
(`shownHiddenModes` at the status; a mode with no box is never hidden in the window), with no migration.

## Consequences

- A user who had hidden Page Functions finds it back after updating, with no message saying so.
- **Covered by** the tests ADR-110 names, each now including `edit`.
