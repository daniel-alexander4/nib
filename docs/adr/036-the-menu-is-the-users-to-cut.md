# ADR-036 — the menu is the user's to cut, and that is a different switch from Advanced features

**Status:** accepted
**Date:** 2026-09-16
**Context:** Dan: *"It should be enabled/disabled in the advanced settings menu. If an item is
disabled in the advanced settings, it should not show in the main menu."* `ADVANCED_SURFACES` and
`applyAdvanced` in `web/app.js`; `test/jsdom/advanced.test.mjs`; `hideableModes` in
`internal/server/settings.go`.
**Applies:** every mode added from now on, and anything that hides a piece of the UI.

## Decision

**A user may switch any main-menu tab off, and a tab switched off is not in the menu.** The set
lives in the vault as `Settings.HiddenModes`, is read at boot from `/api/status`, and is changed
from a **Main menu** card in Settings.

**It is a SECOND, separate mechanism from the advanced-features switch, and fusing them is the
thing this ADR forbids.** They answer different questions:

| | Advanced features | Main menu |
| --- | --- | --- |
| asks | does this feature run at all | do I want this tab |
| enforced | at the door that does the thing (`advancedOn`, 403) | nowhere — there is nothing to enforce |
| granularity | panel and command, **never** the mode | the mode, and only the mode |
| off means | the function STOPS | the tab is not offered |

`web/app.js` has said *"cut at PANEL and COMMAND granularity, never at the MODE"* since
`/pending 451`, and `test/jsdom/advanced.test.mjs` asserts the Signing tab survives ceremonies
being switched off — because co-signing and Simple Sign live in that mode and are not the
ceremony. **That rule is untouched.** Hiding a mode here is the user saying they do not want the
tab; switching ceremonies off there is the user saying the feature must not run. One is a menu
preference and the other is a network-facing capability, and a single control cannot mean both.

So the switch gets **its own card**, next to Advanced features rather than inside it. That card's
own copy reads *"Switching one off stops it — it is not just hidden"*; four more boxes under it
that only hide things would make it untrue.

## Four ways this fails silently, and what catches each

- **Hidden in one list and not the other.** Below 719px `.modetabs` is `display: none` and the
  `[data-modejump]` dropdown is the only way in, so a mode hidden in one list is reachable at one
  width and not the other. `modes.test.mjs` compares list *membership* and cannot see it — both
  lists still hold every id. **One door** (`applyModeVisibility`) takes both, per ADR-009.
- **Landing on a hidden mode.** `setMode` is not reached only by the tabs: `goCard`/`goPanel` jump
  across modes, the marker-fill path sends the user to the Library, the ceremony resume path
  selects Signing. A jump into a hidden mode leaves a pane on screen with no tab above it — the
  same silent shape a missing `SIDEBAR_FOR` entry produces. **`setMode` itself redirects**, so a
  caller written later inherits the rule.
- **Hiding Settings.** The card that un-hides a mode is inside Settings, so this one is
  unrecoverable without editing the vault. `settings` is absent from `hideableModes` and the route
  answers 400; the client drops it too, so a vault written by some future build cannot strand this
  one.
- **The whitelist drifting from the markup.** `hideableModes` is a second statement of the mode
  set. `TestEveryHideableModeIsARealTab` compares it to `web/index.html` both ways, the shape
  `theme.test.mjs` already uses to hold the stylesheet, the Go whitelist and the picker together.

## What it stores, and why that polarity

`HiddenModes` stores what is **hidden**, where `Advanced` stores what is **enabled**. Both pick the
polarity whose dropped key degrades safely, and the safe direction differs: an older build that
drops `advanced` leaves four network-touching features off, and one that drops `hiddenModes` leaves
every mode **reachable**. Storing "visible" instead would make a lost key empty the whole menu —
including Settings.

Empty is stored as absence, per `ViewLayout`'s rule: nothing hidden and never asked are one state.

## What this does not decide

**Whether a hidden mode's commands should be unreachable.** They are not: `Ctrl+O`, `Ctrl+S`,
`Ctrl+F` and the rest still work, and the keyboard is deliberately not cut by a menu preference.
A user who hides File can still save. If that turns out to be the wrong reading of "disabled", the
fix is to make the switch do more, not to make the menu do less.
