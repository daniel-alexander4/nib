# ADR-015 — the toolbar folds whole groups, within their own pane

**Status:** accepted
**Date:** 2026-09-05
**Context:** Dan's small-screen brief; `web/app.js`'s `foldThresholds` / `buildOverflowMenus` /
`applyFold`; `web/index.html`'s `.tbgroup` wrappers; `web/style.css`'s first `@media` rules.
**Applies:** every control added to `#toolbar` from now on. It constrains markup that future
authors will write without reading this, which is why it is a decision record and not a comment.

## Decision

**Every toolbar control lives in a `.tbgroup` carrying a `data-label` and a `data-fold` rank, and
groups fold WHOLE into a `⋯ More` menu built inside their OWN `.tbtab`.**

Three parts, each load-bearing:

- **Groups move; they are never duplicated.** `applyFold` reparents the element.
- **The destination is the pane's own menu.** Mode gating is `#toolbar .tbtab.active`
  (`style.css`) — a *descendant* selector, and there is **no `body[data-tab]` rule anywhere in
  the stylesheet**. A group moved into a `.tbmore` inside its own `.tbtab` is still gated by its
  mode; a group moved anywhere else appears in **all five modes at once**, and nothing looks
  wrong until you change mode.
- **The fold order is fixed at every width.** A command that changed places depending on how wide
  the window happened to be could never be learned.

`data-fold="0"` means never folds. Ranks 1–7 map to widths in `foldThresholds`, pinned by
measurement rather than chosen.

## Why moving, and not the twins this repo already uses

The obvious mechanism was `data-forward` twins — idless duplicates that click through to the real
button, which is this repo's existing answer to one action in two places (`index.html`'s own note,
five in use). It was planned that way and **abandoned during implementation for a concrete
reason: a twin cannot represent a `<select>`**, and the first group to fold (OCR) has two, for
language and quality.

Moving also removes a whole class of defect rather than guarding it. Twins are a second list, and
a second list drifts: a control added to a palette and not to its twin set would simply vanish
below 850px, silently and permanently. Folding by reparenting reads the DOM, so a new control is
folded automatically — provided it is in a group, which is what
`test/jsdom/toolbargroups.test.mjs` exists to guarantee.

## What was measured, because none of it was obvious

Before this change the stylesheet had **no `@media` rule at all** and the entire small-screen
behaviour was `#toolbar { flex-wrap: wrap }`. Chromium against the real binary, Edit palette, one
document open, 768 tall:

| width | chrome before | chrome after |
| --- | --- | --- |
| 1920 | 19.6% | 19.6% |
| 1024 | 28.6% | 28.6% |
| 800 | 33.1% | 28.6% |
| 600 | 42.2% | 24.1% |
| 360 | **63%** | **28.6%** |

The palette grew from 3 rows to **12**; the document was left **14.4%** of a 360px window and is
now 71.4%.

**The horizontal scroll was never the toolbar.** `#menubar` had a hard minimum content width of
**611px** — brand 26 + `.modetabs` 344 + `#statusCluster` 205 — *constant* at 600, 414 and 360.
The sidebar and viewer never contributed, and the `min-width: 0` fix the plan carried for them
would have changed nothing. Below 640 the theme toggle folds into ⚙; below 575 the five mode tabs
become one dropdown. The signature badge stays longest on purpose: it is document *state*, and
burying state in a menu is worse than burying a command.

**Grouping itself costs width** — a divider and a flex box per group — so the fold ladder starts
higher than the pre-grouping numbers implied, and `.tbgroup` must carry `flex-wrap: wrap`. Without
it Annotate's twelve controls became one unbreakable 565px row: a horizontal scrollbar the pane
never had before the grouping meant to fix one.

## What this does not decide

**The five modes are not re-cut.** `/uiux`'s 2026-08-30 review argued they should be — the Library
panel says "use the Sign tab" from inside Sign, and Edit holds OCR, N-up and crop under a verb
promising typing on the page. That is a separate, larger change; this one folds whatever groups
exist and survives it unchanged.

**Group labels are not shown in the bar.** They earn their place as headings inside `⋯ More`,
where they make the fold predictable. Rendering them as captions in the toolbar would add a row of
height at exactly the widths this change exists to shrink.
