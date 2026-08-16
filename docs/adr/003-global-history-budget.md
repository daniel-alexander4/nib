# ADR-003: The undo/redo memory budget is global and bounds the undo+redo pair

**Status:** Accepted
**Date:** 2026-08-16

## Context

Undo history is whole PDFs held in memory. Before several documents could be open at
once, one server held one pair of rings bounded by `maxUndoBytes` (256 MiB), and the
bound counted only the **undo** stack — deliberately, because evicting from redo's far
end silently shortens the user's redo reach.

That reasoning was sound and it had a consequence nobody had written down: undo and
redo are one linear history split at a cursor, so a deep undo could park most of the
bytes in redo where the budget could not see them. The real ceiling for one document
was therefore about **2×** the stated figure.

Moving the rings onto the documents (D8) is what makes this load-bearing. The obvious
translation — each document gets the budget it used to have — turns a 2× overshoot into
**2N×**, growing without limit as the user opens tabs. The memory ceiling would then be
set by a number the user chooses, which is not a ceiling.

## Decision

**`maxUndoBytes` is a single global figure covering every open document's undo and redo
bytes together.** It does not scale with the number of open documents, and it is never
reinterpreted as per-document.

When the budget is exceeded, history is given up in three tiers, in order:

1. Documents that are neither the one that just grew nor the active one — **whole
   histories**, in open order.
2. The active document — whole history.
3. The document that just grew — its **oldest undo entries**, one at a time, keeping
   the last.

An inactive document's history is dropped **whole** rather than trimmed entry by entry,
and a document that loses its history this way records it (`historyEvicted`) and
reports it on the wire.

## Consequences

- **The memory ceiling is a property of Nib, not of how many documents the user opened.**
  That is the whole point, and the reason a future per-document budget must be refused.
- **Whole-history eviction is what makes the budget converge.** A budget covering
  undo+redo cannot be met by dropping undo entries alone: a document whose bytes all sit
  in redo has nothing to give, and a per-entry loop would spin against a ceiling it can
  never reach.
- **Eviction is observable or it is not eviction.** `canUndo:false` reads identically for
  "you have made no edits" and "your edits are no longer undoable", so a partially-
  trimmed history is a silent loss by construction — the user keeps an undo button that
  reaches less far than it did, with nothing saying so. Dropping the history whole is a
  state a document can report; a half-dropped one is not.
- **"The document that grew" is not "the active document", and code that conflates them
  inverts this ADR.** When an operation is addressed to an inactive document, protecting
  the grown one and treating the rest as evictable throws away the history of the tab the
  user is actually looking at. This is not hypothetical: it was written that way first,
  passed every test, and was caught reading the diff — every test grew the active
  document, where the two are indistinguishable.
- It is a bound with **one named exception**, not a hard cap: the grown document always
  keeps its most recent undo entry, so a single state larger than the whole budget
  exceeds it rather than leaving the document unable to undo at all.
- `historyBytesLocked` walks every open document under the server mutex on the mutation
  path. Bounded today by `maxUndoDepth` and a small document count; it becomes a
  hot-path concern when P06 makes that count user-driven, and should then become an
  incrementally-maintained total.

## Alternatives considered

- **Per-document budget (`maxUndoBytes` each).** Rejected — this is the 2N× growth the
  ADR exists to refuse. It is also the shape a future author is most likely to reach for,
  because it reads as the natural consequence of moving the rings onto documents.
- **Keep counting only the undo stack.** Rejected: it preserves the ~2× overshoot and
  multiplies it by the document count. The original reason for the exclusion (protecting
  redo reach) is honoured instead by dropping histories whole rather than trimming redo's
  far end.
- **Trim inactive documents entry by entry, like the active one.** Rejected on two
  independent grounds, either sufficient: it cannot converge (above), and it produces
  exactly the silent partial loss this design refuses.
- **Evict by least-recently-active rather than open order.** The better model, and
  deliberately not adopted: nothing records a last-active moment until document
  switching exists (P06). Approximating it with a signal that would be wrong is worse
  than a deterministic order that is honestly described. Revisit at P06.

## Provenance

Settled as decision D8 in the multiple-open-documents plan, with the undo+redo pair and
the observability requirement added by the plan review's pins. Built as P03.S04
(v1.103.12). The 2× overshoot was found by reading `undo.go`'s existing contract during
the pre-slice deepdive; the active-vs-grown inversion was found in the slice's own diff
review, after the tests were green.
