# ADR-139 — print in an unembedded standard font is boxed to the top of Adobe's box for it

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-098 and ADR-132 (print is boxed by the ink its font's own glyphs reach) to fonts with NO embedded
program. Supersedes ADR-098's guard name `TestPrintInAFontThatIsNotEmbeddedHasNoInk` (rewritten and renamed).
**Context:** /pending 851. After ADR-132, 132 of 6,490 corpus words still had a redaction box reaching the line
above. 103 were in unembedded fonts (ArialMT 35, "Times New Roman" 29, Courier 28, Helvetica 8, Helvetica-Bold 3).
A viewer SUBSTITUTES an unembedded font, and the substitute's glyphs are not Adobe's — so a bound is safe only if
no substitute's ink escapes it.
**Applies:** `internal/pdfops` (`fontinkcore.go`, `coreInk` in `pagemap.go`), the page map's reader only.

## Measured

632 standard-named words; a box reaching the line above / a word's own ink left outside the box, under poppler
with Nimbus | poppler with DejaVu | poppler with Liberation | Ghostscript:

- as it was: 74/0 | 69/0 | 73/0 | 79/0
- per-glyph AFM boxes: 5/0 | 4/**49** (0.9pt) | 5/0 | 5/0 — ink escapes under DejaVu
- the font's Ascender: 9/0 | 8/**45** | 9/0 | 9/0 — ink escapes under DejaVu
- the font's FontBBox top: 43/0 | 26/0 | 28/0 | 50/0

## Decision

1. A simple font with no program, named exactly as one of twelve Core text fonts or a listed alias, gives
   `MapText.Ink` = [the reach's bottom, the font's FontBBox top] (from pdfcpu's `font.BoundingBox`).
2. Only for runs whose codes are all 0x20–0x7E under a named or absent encoding (no `/Differences`).
3. The bottom is never tightened: it moved no word, and DejaVu's tails are deeper than Adobe's.
4. Per-glyph AFM boxes, the Ascender and a tallest-letter bound are refused — each leaks under a substitute.
5. No glyph table is carried, so no data and no notice was added.

`build/accuracy.sh`, before → after: a box reaching the line above 132 → 104, below 53 → 53, ink outside a box
0 → 0, no page worse.

## Declared gaps

- A substitute taller than Adobe's whole-font box plus the 1pt pad would leak; none of the four measured does.
  pdf.js's and Chromium's own substitutes were not measured by pixels.
- Names with spaces ("Times New Roman", 29 words), Symbol and ZapfDingbats get nothing.
- ArialMT gains little (its box top plus the pad is nearly the reach at 8–10pt).
