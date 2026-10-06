# ADR-088 — where things are on a page is read from what the page draws

**Status:** accepted (2026-10-06). Step one of five: the map, its route, and the instrument that measures against it.

## Context

Two features place something on a page by knowing where something else is. *Detect fields* proposes a blank to fill
in; *search-redaction* draws a box over a phrase. Both guessed. Detection rendered the page to an image and scanned it
for dark pixels, using the page's text only to find Yes/No choices. A search hit was placed by measuring its
characters in a generic sans-serif font and rescaling to the text item's width, then padding by 0.8 of a line height
each side. No test at any tier asserted where either landed.

Measured before anything was changed (`build/accuracy.sh`, 19 real documents, 43 pages, v1.185.1):

- **Detection.** On the nine pages whose documents carried real form fields — 682 of them, the answer key — detection
  found 61.4%, and 49.3% of what it proposed was a real field. 27.2% of all proposed text fields sat on printed text,
  and 5% ran through a ruled vertical line. An instructions page with no field on it was given 109. Zooming the page
  changed what was proposed on 5 of 18 documents.
- **Search-redaction.** Every word found was fully covered — and the box ran a mean 2.6 glyph-widths past it, blacking
  out neighbouring glyphs nobody asked for in 72% of cases. 14.8% of the words searched for were not marked at all.

The content stream says exactly where each rule and each glyph is, and reflow's walk (`runWalker`) already read it:
every path operator, and every glyph's advance from the font's own widths. It kept one box per painted path, which is
what reflow needs and not what a form needs — a table ruled by one path is one box.

## Decision

1. **`pdfops.MapPage` is the one door for where things are**: each ruled line and box, each run of text with its glyph
   boundaries, and each form field the document already has (`GET /api/pagemap`).
2. **One coordinate space, converted once.** Everything is a fraction of the page AS DISPLAYED — crop box, `/Rotate`,
   origin top-left — the space overlays are stored in. No caller sees user space.
3. **The walker keeps the pieces of a painted path only for this reader** (`keepShapes`, `readPageShapes`). Reflow's and
   tagging's readers walk past them as before. A piece painted white in a device colour is not in the map; a colour
   the walk cannot name is.
4. **Accuracy is measured, by an instrument that is not a tier** (`build/accuracy.sh`). Its corpus is a local list of
   real documents and is never committed. A document's own form fields are the answer key: the harness removes them
   from a copy, runs detection on the copy, and scores against the fields. It skips cleanly with no list.

## Consequences

- Nothing a user does has changed yet. Detection and search still guess; the following steps move each onto the map
  and are judged by the numbers above.
- **Declared gaps.** The map is of vector content: a scan maps to nothing, and a page whose text is drawn as outlines
  maps to rules with no text. Glyph boundaries are given only for upright runs on an unturned page; anything else
  keeps its box. A stroke's miter and a clipped-away rule are not modelled.
- The corpus is one machine's. "Accurate on real forms" means on these nineteen.
- Cost, measured over 152 documents: median 0.8 ms a page, worst 134 ms.

Guards: `internal/pdfops/pagemap_test.go` (pieces, glyph widths, crop box, the three turns, existing fields, and that
only this reader pays for shapes), `internal/server/pagemap_test.go`.
