# ADR-023 — one undo order for one document

**Status:** accepted
**Date:** 2026-09-06
**Context:** Dan: *"ctrl-z works on drawing lines and it works on drawing shapes, but if I draw
lines and then draw shapes, ctrl-z will not undo the previous set of lines. It should be global
across the document and continue down the full list of changes I've made to that document"*
**Applies:** anything that records an undoable change on the client.

## Decision

**Ctrl+Z walks the document's changes newest-first, whichever stack made them.** Nib has two client
undo stacks — its own overlay commands (shapes, notes, stamps, markers, cover-edits) and pdf.js's
annotation-editor commands (Draw, Text, Highlight) — and each knew only its own. `view.clientHistory`
records which stack each change went to; `undoAny` pops the last entry and asks that stack to undo.
It holds tokens, not commands: each stack still owns its commands, and this decides only whose turn
it is.

**Server operations stay outside it, and that is not an omission.** Every server op reloads through
`setDocumentFromServer` → `clearOverlays`, which drops both client stacks — so "client edits, then
server ops" is already the true chronology and needs no bookkeeping. Recording them would be a
second copy of an invariant the reload already enforces.

**The commit point is `addCommands`, because pdf.js announces nothing else.**
`editingstateschanged` carries booleans, so a second stroke after a first raises no event at all.
`addCommands` is the moment a command enters the manager, and the name survives minification
because it is part of pdf.js's API. The hook is armed from `annotationeditorlayerrendered` — the
manager does not exist before an editor layer does — and it warns loudly rather than degrading,
because a hook that silently failed to install restores the exact defect and is invisible until
someone draws something.

**Ctrl+Z is taken in the CAPTURE phase.** pdf.js binds its own Ctrl+Z inside the editor layer, and
in the bubble phase that listener runs first — which is why the old code yielded whenever one of
pdf.js's tools was armed, and why arming a *nib* tool instead put drawings out of reach. Capture
runs window-inwards, so nib sees the key first and stops it: one press, one undo, one decision.

A text field that has an undo of its own still keeps the key (ADR-020's `ownsUndo`, which includes
pdf.js's contenteditable FreeText body), so undoing *while typing* in a text annotation still
undoes the typing.

## What was measured

Before: two annotation-editor changes on screen, a nib tool armed, four Ctrl+Z presses — nothing
undone, and the Undo **button** reading as disabled throughout. That button counted only nib's own
stack, which is why the routing defect presented as "there is no history" rather than as "the
history is not being walked"; it counts both now.

After: a drawing made first and a note made second, one press removes the note, the next removes
the drawing.

## What is not exercised, stated rather than implied

**A real ink stroke.** No synthetic pointer sequence in this harness produces one — tried as a
Playwright drag and as hand-dispatched `PointerEvent`s, with the editor layer topmost and accepting
events. The test uses a FreeText annotation, which reaches the same manager through the same
`addCommands` door; what goes unexercised is ink's own path to that door, not the ordering this ADR
is about.
