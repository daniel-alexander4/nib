# ADR-138 — a carried tree follows page order for a selection, never for an imposition

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-034 (a subset carries the structure of the pages it keeps).
**Context:** /pending 556. The carry appended kept elements in SOURCE order and nothing re-sorted the root, so a
reordered document's tags read in the order the pages were in before: `Collect(3,1,2)` read pages 2, 3, 1 and
`Collect(1,2,1,2)` read 1, 3, 2, 4. Every element was anchored, the completeness gate was empty and veraPDF was
green; nothing compared the tree's page sequence with the document's. A first attempt sorted inside the carry and
was reverted: `Booklet` selects through the same door, and sorting a booklet's sheets put an H2 before its H1.
**Applies:** `internal/pdfops` (`treeorder.go`, `subsetOnce` and its callers).

## Decision

1. **`treeOrder` is the caller's statement.** `Collect`, `ReorderPages`, `RemovePages` and `DuplicatePage` pass
   `readInPageOrder`; `Booklet` passes `readInSourceOrder`, said at the site — a sheet order is for the printer,
   not the reader.
2. **One level, stable, by first output page** (`orderLevel`).
3. **Only a tree that was already in source page order is sorted**, so two elements swap only where the selection
   turned their pages round. A tree not in page order is left as the author had it.
4. **An element spanning pages the selection separated keeps one place**, by its first output page.
5. **A heading mis-nesting produced by the user's own reorder is the honest consequence** of what was asked.

## Consequences

- Refused: sorting Booklet (7.4.2 t1), and sorting every level.
- A tree not in page order is left untouched silently.
- No census drive reorders, so the reorder's heading consequence has a fixture and no veraPDF row.
- `placeInserted` (`merge.go`) still carries its own copy of the descent; it should call `orderLevel`.
