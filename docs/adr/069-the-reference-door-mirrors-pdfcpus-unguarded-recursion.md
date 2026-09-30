# ADR-069 — the reference door mirrors pdfcpu's unguarded recursion, and bounds its paths and its depth as well as its loops

**Status:** accepted. /pending 764. Extends `/pending 675`'s `/UseCMap` check, which becomes one row of it.

## Context

pdfcpu v0.13.0's validator recurses through a document's references. A Go stack overflow is fatal — no `recover`
catches it — so a reference loop on a chain the validator follows without a guard kills nib with every open
document's unsaved work. `/pending 675` refused one such loop (`/UseCMap`); `/pending 764` measured a second, an
802-byte tiling pattern whose `/Resources` names itself, reached through any open.

The validator's call graph was traced from source (every cycle over `validate/`, each one's guard read, and whether
the mark comes before the recursion). Of the chains a first sweep had listed as unguarded, seven are not (Type 3
`/Resources` and `/CharProcs`, annotation `/AP`, `/SMask /G`, name and number trees, thread beads, page `/Kids`), and
four it missed are (`/Properties` → `/Resources`, type 5 halftones, image `/Mask` and `/Alternates`, renditions and
media clips). The same unguarded chains keep no memo either, so SHARING is walked once per path: tiling patterns each
naming the next twice took 2.8 s to validate at depth 20 from 4.8 KB, doubling per level, and a structure every page
shares is walked again on every page (measured ×70 from one page to fifty, on each of seven resource entries).

## Decision

- **One walk, before the validator, over pdfcpu's unguarded edges** (`internal/pdfread/refgraph.go`), inside the one
  door every validating read already routes through (`Validated`, `Reader`; ADR-009's guard). Each edge is keyed on
  the ROLE pdfcpu reached the object in, because pdfcpu recurses on the role, not the shape. Objects pdfcpu marks
  before recursing — form and image XObjects, fonts, `validateStreamDictEntry` targets, ICCBased streams — stop the
  walk, so a loop pdfcpu handles is not refused. Name and number trees are depth-capped by pdfcpu (at 100), so a
  loop through `/Kids` is not refused either — nib's checker reads such trees as veraPDF does — but their sharing is
  counted, because the cap bounds depth and not breadth.
- **Three refusals, one door:** a loop (`ErrReferenceCycle`, replacing `ErrUseCMapCycle`); more paths than
  2^18 + 16 per object (`ErrReferencePaths`), counted as pdfcpu walks them, once per page for what pages share; and a
  chain deeper than 8,192 (`ErrReferenceDepth`), because depth alone grows the stack.
- **Tested in-process**: one loop per edge, each paired with the same document ending (which must validate), found by
  the walk on the unvalidated read before any loop is handed to the validator; every edge and entry red-proved by
  mutation. No child-process harness.
- `TestTheReferenceDoorNamesThePdfcpuItRestates` fails on a pdfcpu upgrade: the table is re-traced, not assumed.

## Consequences

- 0 of 333 real documents (36 producers, 297 veraPDF corpus) refused; the heaviest used 1.9 paths per object of the
  16 allowed. The walk costs 1.6% of a validated read over that corpus, 2.9% at worst (~0.2 ms on a 7 ms read).
- **Declared:** a quadratic chain pdfcpu really does walk quadratically — a reply thread of roughly 700+ annotations
  (`/IRT`), each validated from the page and each walking the thread — exceeds the budget and is refused.
- **Declared:** recursion inside pdfcpu's `model` package not reached from `validate/` was not traced.
- Six entry edges only feed the path count and have no mutation proof (page `/Annots`, `/HT`, an image's
  `/ColorSpace`, the transfer keys after `/TR`, a shading pattern's `/ExtGState`, a Rendition action's `/R`).
