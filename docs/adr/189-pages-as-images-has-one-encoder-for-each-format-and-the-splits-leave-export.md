# ADR-189 — Pages as images has one encoder for each format, and the splits leave Export

**Status:** accepted
**Date:** 2026-11-02
**Context:** `PLAN-import-export.md` P02.S01 (D25, D26, D27, D28, X10); supersedes ADR-103's "Pages as images" row only; extends ADR-009, ADR-187.

## Decision

- **Export takes a PDF and writes pictures through one encoder per format.** PNG, JPEG and WebP are the browser's own `canvas.toBlob` (pdf.js is the renderer, and Go has no WebP encoder). GIF, BMP, multi-page TIFF, CBZ and ZIP are made by `pdfops.EncodePageImages` from the PNGs the browser rendered, reached only through `POST /api/assemble`'s `format`. The ZIP no longer builds its own archive: it goes through `ziputil.Write`, so the `export.go` exemption in the archive census is gone.
- **The TIFF is written here, not by `x/image/tiff`** (which writes one image per file): an IFD chain, RGB 8-bit, Adobe Deflate, strips of about 256 KiB, a page flattened over white (a transparent pixel must not become black), resolution from the page's width in points. Nib's own reader walks it and Pillow reads it page for page.
- **GIF is 256 colours from a document palette** (40 greys and a 6×6×6 cube), not the stock Plan 9 palette, which dithers a page of text into noise. GIF and BMP hold one picture: two is a 400, never a silently dropped page.
- **A format this machine cannot write stays on the Format line**, selectable, its label ending in the reason; choosing it writes the reason to `#saveAsStatus` and turns Save off (the entry is not `disabled`: a disabled option is skipped by assistive technology). WebP is asked of the canvas when the dialog opens.
- **Each format says what it keeps.** `#saveAsNote` holds one static sentence per format and is the select's `aria-describedby`.
- **The two splits live in Page Functions → Split Pages** with their ids unchanged. A split writes many PDFs; it is not an export of the open document.

## Consequences

Every Format-line label carries its extension. A new picture format is a case in `EncodePageImages` and an entry in the dialog; the caps are the import door's (`importPages`, the per-picture and total pixel budgets) and refuse with 413.

## Declared limits

JPEG and WebP are lossy and of the browser's making; neither is offered for a whole document. SVG waits for P05. Tier 3 (a real browser) drives the card, not the server's pixels.
