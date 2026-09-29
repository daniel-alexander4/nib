# ADR-065 — a per-page loop reads the pages from one walk of the tree, and falls back where one walk cannot answer as pdfcpu does

**Status:** accepted. /pending 753.

## Context

pdfcpu's `ctx.PageDict(i, …)` walks the page tree from the root on every call, so a loop over every page is
quadratic in the page count. nib had written that loop in more than twenty places — the structure gate, the
post-merge sweeps, tag carry, crop, the checker. Measured on tier 4's 7,059-page fixture: `p2p.PrepareDocument` 50 s /
12.9 GiB allocated, 67% of it in pdfcpu's page-tree walk; the quadratic was NOT inside pdfcpu's merge (which splices
two trees) but in nib's own sweeps after it.

## Decision

**`pdfread.Pages(ctx)` is the one door for "every page, in order".** It answers exactly what `ctx.PageDict(p, false)`
answers for every page — the dictionary by identity, its reference, the inherited attributes resolved as pdfcpu's
`checkInheritedPageAttrs` does — from one walk mirroring `processPageTreeForPageDictDepth`, and **falls back to asking
`PageDict` page by page** wherever one walk cannot reproduce pdfcpu: an absent or wrong `/Count`, an untyped node, a
`/Page` with `/Kids`, a `/Pages` node without a direct `/Kids` array, a cycle, depth, attributes pdfcpu would refuse,
or a total that is not `PageCount`. A per-page loop calls it; `livePageObjects` replaces seven hand-copied live-set
loops.

## Consequences

- The fixture: `PrepareDocument` 50.1 s / 12.9 GiB → 12.0 s / 5.1 GiB; `ReadOptimized` 14.8 s → 9.1 s. Output
  unchanged across 36 producers and the fixture (254 rows of ContentDigest, object count, error text).
- Not routed: `ContentDigest`'s own walk (`collectLeaves`) — switching it would change `DocHash` on a tree whose
  subtree `/Count` is wrong (ADR-013 territory); single-page helpers called inside per-page loops (textrun, tagreview,
  tagocr, annotcarry) stay quadratic until their signatures take a `pdfread.Page`; pdfcpu's own
  `optimizeResourceDicts` stays quadratic and is most of what remains.
