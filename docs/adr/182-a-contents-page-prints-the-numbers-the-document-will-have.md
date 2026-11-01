# ADR-182 — A contents page prints the numbers the document will have

**Status:** accepted
**Date:** 2026-10-31
**Context:** `/pending 929`, the second half of `/pending 910`. 910 proposes bookmarks from the headings and has them reviewed in the
outline editor; this adds a page that says what the bookmarks say.

## Decision

- **The entries are the bookmarks, else the headings** (`OutlineFromHeadings`, the reading 910 reviews), or a list the caller
  passes. There is no review step of its own: the review is the outline editor's.
- **The numbers printed are the NEW physical page numbers.** An entry on a page after the insertion is shown k higher, where k is
  the number of contents pages, found by laying out until k is stable. A label is never printed in place of a number.
- **A page label names an index, so it is carried** (`carriedPageLabels`, ADR-046): the new pages carry no label and every other page
  keeps the one it had. A destination names a page object and does not move, so the bookmarks need nothing.
- **The page is artifacts only** — pdfcpu text stamps in nib's own embedded face — so a tagged document gains no untagged content
  (ADR-031). **The entries are not links** in this version: a link on a page of a tagged document must itself be tagged
  (PDF/UA 7.18), and the bookmarks are the clickable form. Linking the entries is a separate slice that must tag them.
- It is a rewrite like every other, so a PDF/UA identification is dropped (ADR-032) and a signed document is refused or loses its
  signature as the page operations do.

## Declared

The tag-fate census (`partial`) reads the new pages as content the tree does not reach, because it cannot tell an artifact from untagged text; they are artifacts, and the verdict is recorded, not enforced, like InsertPDF's.

Titles are cut at a character count (76 less 6 per level), not measured, because pdfcpu positions a stamp by its anchor and nib
does not shape these lines; a long title ends in an ellipsis. There are no dot leaders. One face, one size per kind.
