# ADR-022 — the bar holds what you reach for continuously

**Status:** accepted
**Date:** 2026-09-06
**Context:** Dan, on the fixed toolbar: *"Make suggestions as to what to do with these toolbar
items. Preferably they could be folded into sidebar pills or settings"* — then *"go on this"* on the
proposal below.
**Extends:** ADR-017 (the sidebar carries the commands) and ADR-018 (the sidebar is an accordion).
ADR-017 moved the MODE's commands out of the bar and left the mode-independent ones in it; this
divides that remainder by frequency instead of by mode.
**Applies:** anything added to the toolbar or to File mode.

## Decision

**The fixed bar holds what you reach for continuously; everything else is a card.** The test is
rhythm, not importance. Save, page, zoom and find are used repeatedly while reading one document;
opening, exporting, printing and closing happen once per document.

Moved into File mode as cards: **Open a Document** (Open…, Open & convert to PDF…, and the recent
list), **Save a Copy**, **Export & Print**, **Close Document**. Left in the bar: the sidebar
toggle, Undo/Redo/Reload, **Save**, page, zoom, and find.

**Two groups left File mode in the same change**, because neither was ever a file operation:
*Fill Forms from Data* went to Mark Up and *Combine & Compare* to Document. File mode's sidebar
now means the file's lifecycle, which is what its name says.

**Find collapses to a magnifier.** A text field plus three controls held bar space permanently for
something that is opened deliberately and closed again. Ctrl+F opens and focuses it, Escape closes
it, and closing CLEARS the query — pdf.js keeps its highlights until told otherwise, so a box that
merely hid would leave every match painted with no visible control to clear them.

## The dropdowns are flattened, and that is the point

Recent, Save as and Export were `.menu` popups in the bar. In the sidebar the CARD is the
disclosure, so a dropdown inside a collapsed card is a second disclosure onto the same items — the
pattern the accordion replaced. They are flat lists in their cards instead.

This is also the honest reading of the evidence rather than a preference: **no `.menu` has ever
rendered inside `#commands`** — zero in the mode panes, and no rule in the stylesheet covers the
case. Moving three popups into a 200px scrolling column would have been the untested path.

## What this costs, stated

**Open… is behind a card.** It is the entry point of the whole app, and it is now one header click
away when the sidebar is open, or inside ⋯ More when the sidebar is shut below 900px. The
compensations are that File mode lands on its first card — so Open a Document is the expanded one
on arrival — and that `Ctrl+O` has always existed. If this proves wrong in use, the fix is to put
Open… back beside Save in the bar, not to unwind the rest.

**The tier-3 harness had to learn the cards.** `openDocument` and `closeDocument` click a header
first, through a `card()` helper that opens the sidebar when it is shut — because below 900px the
panes live in the toolbar where the headers are `display: none`, and `group()` would be clicking a
hidden element. The helper toggles the sidebar rather than resizing the window: a helper that
changed the viewport would silently rewrite the state of whichever responsive test called it.

## What this does not decide

**Whether page and zoom belong on the document instead of in the bar.** A floating viewer bar over
the page — the pattern in Preview, Acrobat and Chrome's own viewer — was proposed alongside this
and deliberately deferred: it is the only part that invents a new surface, and the bar is already
one row without it.
