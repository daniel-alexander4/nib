# ADR-118 — Colours is a row of Appearance

**Status:** accepted
**Date:** 2026-10-09
**Context:** Dan: *"combine colours into the Appearance menu item."* Settings had two entries about how Nib looks —
Appearance, a page holding one button (the theme switch), and Colours, a page holding one choice of seven.
`#settingsAppearancePage` and the Settings pane in `web/index.html`.
**Supersedes:** ADR-109's count and list of Settings entries only (seven, with Colours second).
**Applies:** the Settings menu.

## Decision

**1. Settings is six entries:** Appearance, Read Aloud, Toggle Features, Identity & Keys, Vault, About. The Colours
entry and `#settingsColoursPage` are gone.

**2. Appearance holds both, as two named rows:** *Theme* (the forwarded sun/moon switch, ADR-021) and *Sidebar
colours* (the seven radio choices, unchanged — same `name="cardhue"`, same handler, same stored setting). The radio
group is named by its own row title (`#appearanceColoursTitle`), not by the page's.

**3. No collapsible cards here** (ADR-111's pattern): two short rows fit on one screen, and a card would hide a
seven-way choice behind a press for nothing.

## Consequences

- The sidebar shows six coloured entries in Settings, which is exactly the six-step ladder the colour test reads
  (`test/ui/cardhue.test.mjs` asserts at least six) — a seventh entry removed there would break that test's fixture,
  not the feature.
- **Covered by** `test/jsdom/apppages.test.mjs` (six entries in order; the seven choices are on the Appearance page,
  in a group named by its row title; the theme switch is still there) and tier 3's `apppages` (each entry opens its
  page; keyboard order on the Appearance page) and `cardhue` (a choice made on the Appearance page repaints the
  sidebar and survives).
