# ADR-025 — Settings is a mode, and the cards can take one hue

**Status:** accepted
**Date:** 2026-09-07
**Context:** Dan: *"make Settings it's own main menu item and put contents in the sidebar"*, then
*"I'd like the themes page to allow me to choose one those and have the menu be alternating
versions of that one color. Or I could choose an theme that includes all of them."*
**Supersedes:** ADR-021's *"there is no theme picker"* — for the COLOUR axis only. The light/dark
choice is still the sun/moon toggle and still has no list.
**Applies:** anything added to Settings, and anything that colours a sidebar card.

## Decision

**Settings is the sixth mode**, with its items as sidebar cards — Identity & Keys, Vault, Updates,
Appearance, About, Colours. **The ⚙ dropdown is gone rather than kept alongside**: two routes to
one surface is the drift ADR-009 exists to stop, and a gear that still worked would be the one
people kept using.

Nothing in Settings acts on the open document, so none of it is in `DOC_REQUIRED` and each id is a
named exemption — requiring a document would make Settings unreachable on a fresh install, which
is exactly when it is needed.

**Settings → Colours chooses what colours the sidebar's cards**: `all` is the six-accent rotation
ADR-019 measured, and each of the six hues is *that hue at six stepped tints*.

**The ladder is bounded by a number already measured.** Each rung is a fraction of `--card-tint` —
the per-theme level ADR-019 computed (Mocha 28%, Latte 22%, with Latte's red passing by 0.02 at
26%) — evenly spaced from 0.40 to 1.00. So the darkest rung is exactly today's card and every
other is lighter, which moves it toward `--base` and can only raise contrast with `--text`. The
argument is that no new contrast figure is needed; the guard recomputes all thirty-six anyway,
because "lighter is safer" is an argument and not a measurement.

**The value lives on `<html>`, and the stylesheet does the arithmetic.** Nothing computes a colour
in script: the tints are per theme, so a value computed at pick time would have to know which
theme was on and would go stale the moment the toggle was pressed.

**`all` is the ABSENCE of the attribute.** That makes the rotation the default for free, and makes
a vault written before this — or holding a hue that no longer exists — read as the rotation
without a migration, the same rule ADR-021 applies to a retired flavour.

## The hue set lives in three places

The stylesheet decides what a value RENDERS as, the Go whitelist decides whether it can be SAVED,
and the picker decides whether anyone can choose it. `theme.test.mjs` compares the three, for the
reason it already compares the theme lists: each disagreement fails silently and differently — a
colour that applies and is gone after a restart, or one nobody can reach.

## What this cost, recorded

**A guard caught what browser checks did not.** The `SIDEBAR_FOR` entry for the new mode was
missing through several rounds of looking at the rendered sidebar, because `#commands` is a
pass-through that renders regardless — the fallback only bites the CONTENT panels, which Settings
has none of. `modes.test.mjs` named it precisely. A mode that looks right is not a mode that is
wired right.
