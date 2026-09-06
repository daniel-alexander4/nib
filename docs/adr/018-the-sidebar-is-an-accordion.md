# ADR-018 — the sidebar is an accordion of cards, not a tab strip

**Status:** accepted
**Date:** 2026-09-06
**Context:** Dan, on seeing v1.121.0: *"I don't want a single Commands tab with multiple headers.
Make each Header its own one word tab. If there is no good way to do this as tabs, then convert the
sidebar to expandable cards instead."*
**Supersedes:** ADR-017's *"`#commands` sidebar panel"* as a single tabbed panel. The rest of
ADR-017 — the fixed toolbar, the two homes, unrooted gating — stands.
**Applies:** anything added to the sidebar.

## Decision

**The sidebar is a vertical accordion. One card per command group of the active mode, one per
content panel the mode allows, and exactly one card expanded at a time.**

The existing `.tab` buttons *became* the headers rather than being replaced — they move next to the
panel they open and gain `.sbhead` — so their click wiring, their per-mode hiding and `SIDEBAR_FOR`
all keep working. Each command group gets a header built from the `data-label` it already carries.
`#commands` is a pass-through: its cards are the items, so it sizes to content and never claims the
column.

**The sidebar is no longer a tablist.** Its buttons carry `aria-expanded` and sit above the region
they disclose, which is a different widget; `role="tablist"` and `role="tab"` are removed, and the
tablist guard's floor drops from three static surfaces to two (mode tabs, role toggle).

## Why not one-word tabs

Asked for first, and it cannot be done — for a reason that survives any amount of width:

**Document mode has a command group called "Pages" and the sidebar already has a "Pages" tab for
thumbnails.** Two tabs, one word, two meanings. Width finishes it: Document would need seven tabs
across a 200px column, 28px each, against a one-word label needing about 40 at 12px.

The collision is real rather than an artefact of tabs, so it was fixed rather than dodged: the
command group is now **Compose** (extract, insert blank, duplicate, insert PDF), and the thumbnail
panel keeps the word "Pages".

## What the layout cost, all of it measured

Three defects, none of which reading the CSS would have found:

- **The card headers travelled into the toolbar.** They are inserted inside the `.tbtab` panes, and
  ADR-017 moves those panes into `#toolbar` when the sidebar is shut — so the headers went with
  them and added rows to the bar: **33.5% of the viewport at 800px** against a 33% ceiling.
  `#toolbar .sbhead { display: none }`.
- **`#commands` claimed the whole column.** It is a `.panel` and it is `.active`, so a rule giving
  active panels `flex: 1` applied to the pass-through and pushed every content card to the bottom.
- **Each header was 203px tall.** They are flex children of the column with no `flex` set, so they
  stretched into the free space — which is what put a dead gap above every content card. Measured
  from the DOM, not inferred: `#sidebar > .sbhead { flex: 0 0 auto }`.

## What this does not decide

**Which card a mode opens on.** It opens the mode's first command group, by the same rule the panel
list already followed. Whether that is the right one for each mode is a product judgment, and no
assertion can tell a good default from a bad one.
