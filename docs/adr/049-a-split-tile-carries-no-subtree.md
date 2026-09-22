# ADR-049 — a split tile carries no subtree, and the rest of the document keeps its tree

**Status:** accepted · v1.138.14 · the boundary of ADR-047

## Context

`PLAN-ua-coverage.md` P02.S06 was blocked on one question: when `SplitPage` or `SplitRegions` cuts a page
into tiles, should each tile get a clone of the page's structure subtree, so every tile "reads the whole
page", or should the tiles drop the claim?

A tile is a clone of the page dictionary carrying the page's **whole** content stream, shown through a
smaller box. So every tile holds every marked-content sequence of the page, and a subtree cloned onto
each tile would describe all of it — on every tile.

Until P02.S07b the question was also about the whole document: both operations reached the merge through
`splice`, which cut the original with the non-carrying door, so splitting one page destroyed every page's
tags.

## Decision

**A tile carries no subtree** (Dan, 2026-09-21, via /discuss). A clone makes the DOCUMENT read the split
page N times — a two-up spread read as both halves on each half, a 2×2 poster four times — which is a
false account of the document's structure, not a mismatch between the tags and the view. That is exactly
where ADR-047's principle, *a structure tree describes the file, not the view*, stops: it held for a crop
because a cropped page's text is read once.

**The rest of the document keeps its tree** (amended by /grill the same day, delivered by P02.S07b): the
original is the host of `splice`'s one merge, so its tree is carried onto every page it keeps, the split
page's own elements go with the page, and the tiles arrive undescribed under the document's claim. The
fate is `partial` — ADR-031's reasoning: destroying a whole tagged document over one split page is the
cure being worse than the loss.

## Consequences

- `SplitPage`/`SplitRegions` are `partial` on any document with more than the split page; on a one-page
  document the split page is the whole tree and the result is `dropped`, as the census's one-page fixture
  shows.
- veraPDF: the tiles' content is untagged (7.1 t3). Where the split page carried the only `/H1`, the rest
  of the document starts at `/H2` (7.4.2 t1) — `RemovePages`' recorded reason, one operation over.
- **Not done:** a positional partition — each tile keeps the elements whose content lies inside it and
  turns the rest of its (shared) content into artifacts. The per-element extent (`structview.go`) and the
  artifact edit (`structartifact.go`) exist; rewriting each tile's content so it no longer shares one
  stream is the unbuilt part. It shares that geometry with `/pending 577` (a crop's clipped elements).
