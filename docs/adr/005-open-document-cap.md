# ADR-005: The open-document cap bounds count **and** aggregate bytes

**Status:** Accepted
**Date:** 2026-08-17

## Context

Holding several documents at once needs a refusal point, because each open document
costs its bytes (up to `maxPDFBytes`, 200 MiB), a pdf.js proxy, a rendered DOM
subtree, and a share of the undo budget. The first form of the cap was a count:
eight documents, refused with a clear message rather than degrading.

**A count cap does not bound the thing it exists to bound.** Eight documents is a
number; eight *documents* is anywhere from a few hundred kilobytes to **1.6 GB**,
because nib accepts documents up to 200 MiB and real ones vary by three orders of
magnitude. A count-only cap is therefore either far too loose (eight large scans) or
pointlessly tight (eight forms), and which one it is depends entirely on what the
user happens to open.

## Decision

**The cap is count AND aggregate bytes, refusing on whichever binds first.**

- `maxOpenDocs` = **8**.
- `maxOpenBytes` = **512 MiB** of aggregate `doc.data`.

**The byte figure was chosen against a measurement, not assumed.** The method, and
the parts of it that decide the constant:

- Eight documents at `maxPDFBytes` is **1.6 GB** of `doc.data`. That is the exposure
  the count cap alone leaves, and it is the number this bounds.
- **512 MiB admits eight ordinary documents** — eight 60 MB scans is 480 MB — and
  refuses only the pathological set. A cap that refuses real work is a cap users
  route around, and a routed-around cap bounds nothing.
- It is **2× `maxUndoBytes`**, so the two server-side bounds are the same order and
  the whole server ceiling sits under a gigabyte, which is a number a person can
  hold in their head while reasoning about the process.

**It counts `doc.data` and nothing else, deliberately.** The undo and redo rings are
already bounded by ADR-003's global `maxUndoBytes`. Counting them here would make two
bounds fight over one pool: a document could be refused because *another* document's
history was deep, which is a refusal the user cannot act on and cannot understand.

## Consequences

- Anyone raising either figure is raising a memory ceiling, and the two are related:
  `maxOpenBytes` at 2× `maxUndoBytes` is the relationship that keeps the total
  legible. Change one and state what happens to the other.
- The refusal is a first-class outcome with a message, not a degradation. A cap that
  degrades — dropping the oldest document, say — loses work the user did not choose
  to lose.

## Alternatives considered

- **Count only.** Rejected above: it does not bound memory at all.
- **Bytes only.** Rejected: the per-document costs that are *not* bytes (a pdf.js
  proxy, a DOM subtree, a thumbnail grid) scale with the count, so a hundred tiny
  documents is a real cost a byte cap cannot see.
- **Counting undo/redo bytes in the same budget.** Rejected — see above; two bounds
  over one pool produce refusals nobody can act on.

## Provenance

Settled as decision D9 in the multiple-open-documents plan (a count, amended to count
+ bytes by that plan's dimension review), with the byte figure measured at P06.S04.
The plan is retired; this ADR is the surviving record of the measurement's method,
which `internal/server/server.go` cites at `maxOpenBytes`.
