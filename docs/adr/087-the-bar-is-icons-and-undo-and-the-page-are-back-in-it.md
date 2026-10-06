# ADR-087 — the bar is icons, and Undo and the page position are back in it

**Status:** accepted (2026-10-06). Supersedes ADR-024's "the toolbar loses Undo/Redo and Previous/[n]/Next". Extends
ADR-015 (groups fold whole) and ADR-022 (the bar holds what you reach for continuously).

## Context

The fixed bar had grown to ten text buttons in one group (Pages … Zoom in) beside a text Save and Quit, and showed
neither the page, nor the zoom level, nor any way to undo. ADR-024 had removed Undo/Redo and Previous/Next on the
ground that Ctrl+Z and PageUp/PageDown reach them, and named the cost itself: *"a user who does not know the shortcut
cannot undo"*, and the standing hint that earlier history had been released went with the button (`/pending 423`, 462).
Dan asked (2026-10-06) for every bar control to be an icon with a tooltip, for both to come back, and for controls
that apply only sometimes to appear only then.

## Decision

1. **Every control in the fixed bar is an icon, and its word stays in the markup.** A `.tblabel` span is clipped to
   nothing in the bar and shown inside ⋯ More. It is the button's accessible name, the label a folded group shows
   (ADR-015 moves a group whole, so an `aria-label` alone would arrive in the menu as a bare picture), and the text
   a test finds a button by. Nothing may assign a bar button's `textContent`: Read aloud writes its label span.
2. **Undo and Redo are in the bar**, disabled when there is nothing to undo and never hidden — a control that comes
   and goes moves everything beside it. `reflectUndoControls` is their one writer. While the server has released a
   document's history and nothing is left to undo, the Undo button is outlined and its tooltip says so: the standing
   home ADR-003's eviction notice lost at v1.125.0.
3. **Previous, the page field and Next are in the bar.** The Pages tab keeps its readout; both are written by class
   through the one existing door, so they cannot disagree.
4. **Zoom is its own group again**: out, the zoom level, in, Fit width, Fit page. The level is a button that returns
   to 100% — it keeps `actualSizeBtn`'s id. **No popup**: a dropdown inside a group that folds is a menu inside a
   menu, which the bar's one-open-menu rule and `toolbargroups.test.mjs` both refuse.
5. **One contextual group**: while a tool owns the pointer the bar names it and offers a button that puts it down
   (`reflectArmedChip`, driven from `reflectPanCursor` — the same fact). The Reload icon is outlined while the file
   on disk differs.
6. **The fold ladder is re-measured, and it is the fixed bar's alone.** Widths and sums are written beside
   `foldThresholds`. What goes first: View, Page, History, Zoom, Find. The title's cap drops from 38ch to 22ch, because
   the ladder is summed against it and the tab strip below carries the whole name.

## Consequences

- **Declared gap: whether sixteen pictures are read as easily as sixteen words is untested**, and no tier can test it.
  Tooltips appear on hover; a keyboard or touch user has the accessible name and nothing visible.
- While a tool is armed, in the ~110px above each fold rung, the bar takes a second row.
- Not built, and filed rather than faked (`/pending 849`): showing Read aloud only when the page has text, form tools,
  actions on a text selection, a hand tool, and a tagged/untagged indicator. None has state behind it today; each is a
  feature of its own.
- Quit stays in the bar (Dan's call).
- The refused alternatives: `aria-label` on icon-only buttons (bare pictures in More, and every text-matching test
  rewritten); a Fit popup under the percentage (decision 4); hiding Undo/Redo when empty (decision 2).

Guards: `test/jsdom/toolbaricons.test.mjs`, `test/ui/toolbaricons.test.mjs` (one row at nine widths, the word hidden
in the bar and shown in More, the zoom level against the page), `test/jsdom/viewmodes.test.mjs` (the two groups),
`test/jsdom/doccontrols.test.mjs` (the three named exemptions).
