# ADR-071 — text that moves takes what is anchored to it, and its structure, or refuses

**Status:** accepted. `PLAN-text-reflow.md` P07 (closed 2026-09-30). Extends ADR-031 (nothing claims tagging it has
not) and ADR-009 (one door).

## Context

P07 made a reflow stop being local: a paragraph that needs more lines pushes the paragraphs below it down, and what no
longer fits flows whole onto the next pages. Everything a page fixes by position — annotations and their popups, form
widgets, destinations (links, bookmarks, named destinations, `/OpenAction`, `GoTo` in any action or `/Next` chain),
article beads, NibFlags, and the structure tree's MCIDs and OBJRs — then either moves with the text or points at the
wrong words. The phase-close review found every way that rule can be broken quietly: anchors re-selected by zone AFTER a
move (so a kept link was carried off its page), a flow that jumped content staying below it (reading order inverted), a
re-set line run under a widget, destinations in widget and additional actions never collected, marked content without
an MCID (`/OC`, `/Lang`) dropped on a carry, invisible OCR text moved off its scan.

## Decision

1. **A move is planned whole, then applied.** Every anchor a flow moves is selected from the ORIGINAL page, at plan
   time, and applied by identity (`anchorSet`); nothing is re-selected after a mutation. An anchor that cannot move
   exactly — straddling a moving edge, crossing to a page turned differently, a bead — refuses `anchored`.
2. **A flow leaves a page only past its margin.** Paragraphs go to the next page only when nothing but margin lies
   below them, and land only in a next-page region the margin bounds; otherwise `page-full`. Content below the text —
   a signature block, a footnote, a running header — is never jumped.
3. **A carried paragraph takes its structure.** Its marked content is re-opened on the target page under its own tag
   and property list at an MCID free there; the element's kid is REPLACED IN PLACE by an MCR naming the target (a
   sequence split across the pages gets a second MCR after the first); both `/ParentTree` rows say so; a carried
   annotation's OBJR follows it. What cannot be carried correctly refuses `tagged-across-pages` — never carried
   untagged (ADR-031).
4. **The structure write half writes nested `/ParentTree`s** — a key some node holds, in place; a new key above every
   key, at the end of the rightmost leaf with every `/Limits` on the path widened — and refuses any other placement.
   This REVERSES the write half's earlier blanket refusal, which reached 7 of the 14 multi-page real-producer
   documents. `parentTreePlace` is the one predicate both the writer and the carry's preflight ask.
5. **A page's `/StructParents` has one reader** (`structParentsOf`), resolving an indirect value: reading it as a bare
   integer re-keyed pages whose key was a reference.

## Consequences

- Every refusal is a named cause the dialog words; a flow that cannot honour rule 1–3 changes nothing.
- The margin rule refuses some documents an earlier cut flowed (a next page whose first region is bounded by content
  below it). Accepted: inverting reading order silently is the worse failure.
- The subset carry (`structcarry.go`) still refuses a nested tree itself — built and measured over flat trees only
  (`/pending 528`, gate now met).
