# ADR-206 — A PowerPoint file is written from the page reader, one slide a page, and is the one writer outside docwrite

Status: accepted, 2026-11-04 (PLAN-import-export P05.S03, v2.47.0). Extends ADR-193 (Document as…), ADR-088 (the page map's fractions), D21, Y3.

1. **Positioned, not flowing.** `internal/pptxexport.Write(pdfops.ExportDoc, title)` makes one slide per page: a text box where the page sets each
   paragraph and each table cell's paragraph (`ExportParagraph.Rect`, fractions of the displayed page, and `Size` in points), and a picture
   (`p:pic`) or, for a placeholder, an outlined box saying so, where the page draws one. Bold and italic are kept; face and colour are not (the
   map holds neither); links are dropped and counted. A table is its cells' boxes, not a PowerPoint table.
2. **It is not a `docwrite` package** (Y3). The docwrite writers take the flowing model and may not reach a PDF reader; a slide needs boxes, so
   this package reads `pdfops.ExportRead` and is the census's one named exception. `TestThePowerPointWriterReachesTheExportReaderAndNothingThatSignsOrServes`
   holds what it may reach (with the control that it does reach `pdfops`), and `TestOnlyTheExportRouteImportsThePowerPointWriter` that exactly one
   package, the server, imports it.
3. **The visibility rule is the reader's, not a copy.** `ExportRead` applies `visibleRuns` before it groups, so covered, hidden and off-page text
   reaches no slide; the route fills the same `X-Nib-Export` counters from the same `ExportVisibility`. (Y3 asked for "one shared function"; there
   is one, because the box fields were added to the export read rather than a second reader built.)
4. **The route.** `POST /api/export/document?format=pptx` takes a branch in `handleExportDocument` (`exportFormats[..].slides`) that keeps the
   snapshot, the revision check and the 409 and 422 answers; an extra 422 `too-many-pages` past `MaxSlides` (2000). A document with no text, table or
   picture on any page is 422 `empty`. A scanned page's picture is placed and the page is listed in `scannedPages`.
5. **Slide size** is the first page's, capped at PowerPoint's 56 inches; a later page of another shape is fitted inside it, centred, never
   cropped or stretched. A box is widened 6 percent and one point, and held inside the slide, because the program's substitute face is wider.
6. **Confirmed by running LibreOffice**: Impress opens the file and a PDF made from it has one page per slide with the words in order
   (`TestImpressReadsItBack`; skipped by name without `soffice`). PowerPoint itself is unrun.
7. **Declared:** the fonts and colours of the source are not carried (the map holds neither); a rotated or skewed run sits in an upright box; a
   table is not a PowerPoint table; links and the document's tags are not kept; the overlay the window has not baked is not in it (W8); PowerPoint,
   Keynote and Google Slides are unrun.
