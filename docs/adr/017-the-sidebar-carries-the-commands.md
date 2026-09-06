# ADR-017 — the sidebar carries the commands, and the toolbar stops changing

**Status:** accepted
**Date:** 2026-09-06
**Context:** `/pending 373`; `web/index.html`'s `.tbfixed` bar and `#commands` panel;
`moveCommandsHome` / `applyFold` / `foldAll` in `web/app.js`; `SIDEBAR_FOR`.
**Supersedes:** ADR-015's *"a `⋯ More` menu built inside their OWN `.tbtab`"* and ADR-016's
*"mode gating is `#toolbar .tbtab.active`"*. Both were true and are no longer.
**Applies:** every control added to the toolbar or a mode from now on.

## Decision

**A mode's commands live in the sidebar. The toolbar holds only what is true in every mode.**

| Surface | Holds | Changes with mode? |
| --- | --- | --- |
| `#toolbar .tbfixed` | Open · Save · Page · Zoom · Find · Close, plus Undo/Redo | **no** |
| `#commands` sidebar panel | that mode's groups, vertical, with visible headings | yes |
| each pane's `⋯ More` | the mode's groups, when the sidebar is shut | fallback |

**"Fixed" means it does not change with the MODE. It still folds with the WIDTH.** Those are
different axes and conflating them was measured wrong: six unfoldable groups wrapped to **34.8% of
the viewport at 800px** against the 33% ceiling — worse than the 28.6% ADR-015 achieved.

**Mode gating is now `.tbtab.active`, unrooted.** A pane lives in `#commands` while the sidebar is
open and in `#toolbar` while it is shut, so a selector rooted at either passes or fails on which
state the page happened to boot in.

**Panes are all-or-nothing.** In the sidebar nothing folds — the column scrolls. In the toolbar
everything folds, whatever the width, so the bar shows the fixed commands and one `⋯ More`. The
width ladder now governs `.tbfixed` alone; a ladder over the panes would fight `moveCommandsHome`
and could leave a pane half in each home.

## Why

The measured problem ADR-015 was written for is gone: the Edit palette ran 29% of the viewport at
1024×768 in August and **14.1% at 1366** after v1.120.0. What survives is legibility — ADR-015
deliberately keeps group labels *out* of the bar because a caption there costs height at exactly
the widths that are tight, so at the widths this product is used at a palette is an unlabelled run
of controls.

A 200×580px column has the room a horizontal bar does not. Measured: a fully vertical layout of
each pane is Mark Up **318px**, Document **211**, File **175**, Secure **175** — every one fits
without scrolling. The cheap alternative, captions above still-horizontal groups, was measured too:
File 108→**164px**, Mark Up 39→**67**, Document 108→**226**, Secure 108→**164**. Feasible, but it
*grows* the bar, which is the opposite of the brief.

And a toolbar that never swaps retires the 2026-08-30 walk's first finding — *five mutually
exclusive palettes* — which a merely-smaller per-mode bar would keep.

## What this cost, recorded because none of it was predicted

The plan's blast radius was reasoned, and tier 3 went from 75 passing to **39 failing**. Four
corrections, each finding something the last did not:

1. **`#closeBtn` became invisible.** Close stayed in the File pane, which is now a sidebar panel
   shown only when its tab is active — and the tier-3 harness closes documents from every file. One
   hidden button, 22 failures. Closing is the counterpart of opening: it is mode-independent.
2. **Switching mode did not switch the panel.** `syncSidebarForMode` only changed panel when the
   showing one became *invalid*, and `thumbs` stays valid for most modes — so you picked Secure and
   got thumbnails, with its commands behind an unmarked click. It now always lands on the mode's
   first panel.
3. **The armed-tool indicator followed the panel, not the document.** Four sweeps were rooted at
   `#toolbar` (`[data-mode]`, `[data-forward]`), so with the tools in the sidebar they silently
   stopped reaching them. A real defect, caught by a test that had been green for months.
4. **`Open` could not fold** — it carries the `Recent` dropdown, and a menu inside `⋯ More` is a
   menu in a menu. Rank 0, like `Save`.

**Collaborate does not land on Commands.** Flags is a documented product decision that eight tier-3
tests protect; Commands is a sibling panel there. The inconsistency is deliberate.

## What this does not decide

**Whether landing on Commands is right for every mode.** Opening a document now shows a command
list rather than page thumbnails, and per-page thumbnail work costs a click back to Pages after
every mode change. That is a product judgment, taken as the plan's stated default, and no assertion
can tell a good default from a bad one — only that it is stable.
