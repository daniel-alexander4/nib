# ADR-024 — the sidebar has two sections, and the bar names the document

**Status:** accepted
**Date:** 2026-09-06
**Context:** Dan: *"give the sidebar two tabs. The first is Pages with the content of the current
Arrange Pages pill. The second Functions which contains the rest of the current pills minus Arrange
Pages. Remove Undo/Redo and the Previous <count> Next from the toolbar. Right justify Zoom, Reload
and Save."* Then: *"The title of the document should appear in the tool bar, left justified. It
should have a small indicator of whether it has been modified or not and needs to be saved."*
**Extends:** ADR-018 (the accordion) and ADR-022 (the bar holds what you reach for continuously).
**Applies:** anything added to the sidebar or the toolbar.

## Decision

**The sidebar is two sections.** *Pages* is the thumbnail grid alone; *Functions* holds the
accordion of command cards and the remaining content panels. The accordion is unchanged and now
lives inside a tab.

**ADR-018's objection does not reach this strip.** It refused one-word tabs for all seven surfaces
on width — seven across 200px is 28px each against a label needing about 40. Two is 100px each. It
is a real tablist this time (`wireTablist`: roving tabindex, arrow keys, `aria-selected`), which is
the contract the tablist guard discovers by role and enforces.

**Functions is the section a mode change selects**, because choosing a mode is saying what you want
to do — the same reasoning that made a mode land on its own commands. Pages is a choice the user
makes and keeps until the next mode change.

**The toolbar loses Undo/Redo and Previous/[n]/Next.** Ctrl+Z is undo's route and ADR-023 made it
reach every change in the document; PageUp/PageDown/Home/End page, and the thumbnails are a better
target than a Next button. The page-number readout moved to Pages rather than being deleted — it is
the one part of that group that reports rather than acts.

**Zoom, Reload and Save are pushed right; the document title sits at the left edge.** The title
carries a save-state dot that reads `hasUnsavedWork` — the same flag the close prompt asks. It is
NOT inside a `.tbgroup`: groups fold into ⋯ More at narrow widths, and a bar that hides which file
you are editing is worse on a small window than on a large one.

**The unsaved flag gets one door.** `setDirty(owner, value)` assigns and repaints. Ten sites wrote
`dirty` directly, and a display painted at some of them was wrong in a way that looked like a
rendering bug and was not: a *freshly opened* document read "Unsaved changes", because the
universal document sink marks every arrival dirty and `installOpened` corrects it a line later.
The bar and the close prompt now cannot disagree.

## What this cost, named rather than hidden

**Undo has no visible control at all now.** Ctrl+Z is the whole interface. Two consequences: a user
who does not know the shortcut cannot undo, and the standing hint that earlier history had been
evicted (ADR-003) went with the button's tooltip — though the eviction TOAST remains, so the fact
is still announced. The red proof asserting that the button reflected the editor stack was retired
with the button; it asserted a control the product no longer has, and a row that cannot fail is
worse than no row.

**A test observable went with it.** `gestures` used the button's enabled-ness to detect a command
recorded onto the wrong document. It now asserts the CONSEQUENCE its own message always named —
press Ctrl+Z on the document that was switched to, and nothing on the other document may move.

## What the harness had to learn

`panel('thumbs')` is a tab click, and it waits on VISIBILITY rather than on a class — a class is set
whether or not the sidebar is on screen, so with the sidebar collapsed the helper returned happily
and the caller's next hover timed out thirty seconds later inside a hidden column. The old selector
could not fail that way, because Playwright refuses a hidden click target; the silent success
arrived with the tab. `showSidebar()` is now one door for both helpers, and `gotoPage(n)` is one
door for typing a page number, which seven call sites had been doing directly.
