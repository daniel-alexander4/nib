# ADR-077 — the reference door bounds depth through the edges pdfcpu guards, too

**Status:** accepted. `/pending 803` (2026-10-03), from the P02 phase-close review of `PLAN-returned-document.md`.
Extends ADR-069: its walk still stops at a guarded object for loops and paths; a second, depth-only pass does not.
**Superseded in part by ADR-081** (the `((n+2)/2)²` charge for a cyclic component).

## Context

ADR-069's walk stops at every object pdfcpu marks before recursing — form and image XObjects, Type 3 fonts, soft-mask
groups, appearance streams — because pdfcpu handles a LOOP there and validates the object once. Once is not shallow:
a loop-free chain of forms, each naming the next in `/Resources /XObject`, is validated one recursion per link
(`validateXObjectStreamDict` → `validateFormStreamDict` → `validateResourceDict` → …). Measured through `Validated`:
300,000 links (51 MB) 9.5 s and returned; 700,000 links (~120 MB, inside ADR-005's 512 MiB) `fatal error: stack
overflow`, which no recover holds, so the process went with every open document.

## Decision

- **A second pass bounds the depth over every edge pdfcpu follows, guarded or not** (`internal/pdfread/refdepth.go`,
  `validatorDepth`, run by `refuseUnboundedReferences` after the path count). The guarded edges are added to the same
  edge table behind `refGraph.guarded`, so the path count never follows them.
- **It is an upper bound over every order pdfcpu could take**, not a simulation of one: pdfcpu ranges over Go maps, so
  which object it reaches first varies. Strongly connected components (Tarjan, iterative) are charged
  `((n+2)/2)²` — a stack inside one can revisit an unguarded node once per guarded object it passes — and the depth
  is the longest path through their condensation.
- **Past ADR-069's 8,192 it is `ErrReferenceDepth`**, the same refusal and bound as the unguarded walk's.

## Consequences

- 0 of 2,536 real documents refused (nib's producer corpus, the veraPDF corpus, TeX Live's and the Go module cache's
  PDFs); the pass costs about what the path count does (726 ms against 737 ms over that corpus; 31 ms at worst).
- **Declared:** the guarded edges are those that carry resources onward — `/Resources /XObject` (forms and images),
  `/Resources /Font` (Type 3), an ExtGState soft mask's `/G`, an annotation's `/AP` streams and an image's `/SMask`. A
  guarded object with no onward edge (a font program, an ICC profile) is a leaf and does not deepen anything.
