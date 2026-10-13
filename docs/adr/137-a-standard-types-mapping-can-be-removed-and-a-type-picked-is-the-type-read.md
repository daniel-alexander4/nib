# ADR-137 — a standard type's mapping can be removed and never set, and a type picked for a tag is the type read

**Status:** accepted
**Date:** 2026-10-12
**Amends:** ADR-127's "a standard type is never the name", for removal only. **Extends:** ADR-124, ADR-125.
**Context:** /pending 632 part 7. A document's role map may map a STANDARD type to another (`/P → /Span`; some
producers write it). Retyping an element to that standard name then means something other than the reviewer
chose, silently: the tree says `P` and a reader resolves `Span`. ADR-127 let the role map edit name only custom
types, so such a mapping could be neither set nor removed.
**Applies:** `internal/pdfops` (`structrolemap.go`, `structedit.go`, `structcreate.go`, `structregion.go`), the
Tags panel.

## Decision

1. `rolemap` with an empty value removes a mapping whose NAME is a standard type, whatever tags use it — a
   standard type explains itself, so nothing is left unmapped. Setting one stays refused (ua1 7.1 t7).
2. `typeAsChosen` is the ONE check for retype, create and region: a standard type the document's role map sends
   elsewhere is refused, naming the mapping and the remedy (remove it in the role map).
3. In the panel, the row for such a mapping offers Remove only.

## Consequences

- A type picked in the editor is the type a reader resolves, or the edit is refused with the way to make it so.
- An element ALREADY typed with a remapped standard name is not judged by this — the checker's 7.1 t7 is.
