# ADR-019 — a theme lives in four places, and its card tint is measured per theme

**Status:** accepted
**Date:** 2026-09-06
**Context:** Dan: *"Give the cards a nice color. grey is too drab. color should be happy. Give me a
way to select the color theme in the settings page."* `web/style.css`'s palette blocks;
`internal/server/settings.go`'s whitelist; the picker in `web/index.html`; `THEMES` in
`test/jsdom/theme.test.mjs`.
**Applies:** anyone adding a theme, or changing what a card is coloured with.

## Decision

**Nib ships four Catppuccin flavours — Latte, Frappé, Macchiato, Mocha — chosen from the ⚙ menu.**
`light` stays Latte and `dark` stays Mocha: those two values are already persisted in people's
vaults, and renaming them would silently reset everyone's theme.

**A theme exists in four places and all four must agree:**

1. the token block in `web/style.css`,
2. the whitelist in `internal/server/settings.go` — which decides whether the choice can be *saved*,
3. the picker's radio values in `web/index.html`,
4. `THEMES` in `test/jsdom/theme.test.mjs` — which decides whether it is contrast-checked *at all*.

Each disagreement fails silently in its own way: an unguarded palette, a choice that applies and is
gone after a restart, a flavour nobody can pick, a palette no test reads. `theme.test.mjs` now
compares the four lists, the way `modes.test.mjs` compares the four lists that describe a mode.

**A sidebar card's colour is one of the six accents at `--card-tint` over `--base`, and the tint is
a per-theme token.**

## Why the tint, and why per theme

Two simpler schemes were measured and refused before this one:

- **Accent-coloured header text** passes in Mocha (5.43–9.89:1 on `--surface0`) and **fails all six
  accents in Latte** (1.70–3.52). Latte's accents are accents *on* light, not text, and `--text`
  (#4c4f69) is already its darkest token — no contrast is recoverable from the text side.
- **A coloured left rail** needs 3:1 and only blue, red and mauve clear it on light `--surface0`;
  green 2.17, yellow 1.70, peach 1.93 do not.
- A tint over `--surface0` fails too — light red is 3.92 at only 18%.

Over `--base` it works, but **not at one level for every flavour**, which is the part that had to be
found rather than assumed. Each theme's own base decides how much accent it can carry before the
worst pair drops below the floor. Computed, holding the worst accent at **≥ 4.8:1** so the margin is
real rather than hundredths:

| flavour | tint | worst pair |
| --- | --- | --- |
| Mocha (`dark`) | 28% | yellow 5.07:1 |
| Macchiato | 26% | yellow 5.02:1 |
| Frappé | 22% | yellow 4.82:1 |
| Latte (`light`) | 22% | red 4.86:1 |

**Frappé is the constraint and it is not obvious**: its base (#303446) is the lightest of the three
dark flavours, so it has the least headroom. A single 30% tint passed Mocha and Macchiato and put
Frappé's green at **4.16** — caught by the guard, not by looking.

**Accents rotate in order, not by a hash of the label.** A hash survives a group being added, which
an index does not, and it was tried first — then rejected on looking at it, because Compose and
Page setup came out adjacent pinks. With at most six groups in a pane the rotation gives every card
its own hue, and that is the property worth having.

## What this does not decide

**Whether six hues read as happy or as busy.** That is taste, it was settled by looking at the
rendered column in all four flavours, and no assertion can hold it.
