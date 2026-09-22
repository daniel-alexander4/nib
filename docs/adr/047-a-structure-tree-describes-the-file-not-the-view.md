# ADR-047 — a structure tree describes the file, not the view

**Status:** accepted · v1.138.11

## Context

`PLAN-ua-coverage.md` P02.S05 was blocked on one question: *may a structure tree describe content a
crop has clipped from view but not removed?* Crop dropped the whole tree, because it rebuilt each
target page through pdfcpu's `CutPage` and merged the pages back — and `CutPage` also deletes every
annotation on the pages it rebuilds, so a crop silently destroyed links, comments and form fields as
well as the tags.

The phase-open deep-dive (2026-09-15) had measured that a crop applied in the source document is
veraPDF-compliant with the tree intact, even at a 5% window. So the question was never whether the
tags could survive — it was what a reader should hear.

## Decision

**A structure tree describes the FILE, not the view** (Dan, 2026-09-21, via /discuss). A crop hides;
it does not remove. The clipped text is still in the page's content stream: it extracts, copies and
searches exactly as it did before the crop. Tags that go on describing it describe something true, so
keeping them breaks no part of ADR-031's law 1 — and dropping them lost every visible element's
structure to avoid a mismatch on the hidden ones, which is the cure being worse than the loss.

**So Crop moves each target page's boxes and nothing else** — the window becomes its `/MediaBox` (and
its `/CropBox`, where it has one), and the printing boxes, which must lie inside the MediaBox, go. The window is computed in the
page's display space (its CropBox, turned by its `/Rotate`) because that is what the user drew on,
and written in the page's own space, so the content stream, the rotation, `/StructParents`, every
MCID and every annotation `/Rect` keep their meaning without being touched. The tree is carried
without a carry. An inherited `/CropBox` is overridden explicitly rather than deleted, since deleting a
key the page never had leaves the ancestor's larger box in force.

## The boundary — where this principle stops

It holds because a cropped page's text is read **once**. P02.S06 (splitting one page into tiles) is
the case it does not license: every tile carries the whole page's content, so cloning the page's
subtree onto each tile would make the *document* read that page N times — a false account of its
structure, not a view mismatch. The split tiles carry no subtree (decided the same day); that
decision's ADR states the other side of this line.

## Consequences

- `tagFates` declares `Crop` `carried`; the veraPDF differential's `Crop` row, five `pageSetLoss`
  clauses, is gone.
- **Annotations on a cropped page survive**, where they previously went with the rebuild. One outside
  the window is off the page and hidden, like the content under it.
- Output pages keep their `/Rotate` and may have a MediaBox whose origin is not `(0,0)`. Both were
  already true of documents nib opens and of `SplitPage`'s tiles; `p2p/cosign.go`'s `fitToPage`
  already places onto an offset box.
- The generated `ContentDigest` golden's `cropped` row moved because its INPUT changed; the other
  rows of both goldens did not, which is the evidence the digest function did not.
- **Not done here:** turning elements wholly outside the window into artifacts. The per-element
  extent (`structview.go`) and the artifact edit (`structartifact.go`) both exist; the unbuilt part is
  applying one to the other. Filed as a follow-up rather than built, because whether a clipped
  element should be heard is the refinement, not the rule.
