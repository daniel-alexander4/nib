# ADR-094 — a page that already has a text layer is not OCR'd again

**Status:** accepted (2026-10-06). Closes /pending 851 part 4's harm; the replacement it leaves open is named below.

## Context

Nothing recognised an existing OCR layer. Running OCR on a document that had been OCR'd stamped every word a second
time (measured: one word, one invisible run after the first pass, two after the second). Search found each word
twice, a copy carried it twice, and a file stamped before ADR-092 kept its short words under the fitted ones — so
"OCR it again" could not be given as the way to bring an old file up to date. Neither the route nor the window checked.

## Decision

1. **A page that already carries invisible text gets no second layer.** The rule is one function,
   `pdfops.PagesWithTextLayer`: a page whose map has a `Hidden` run. It does not ask whose layer it is — another
   tool's doubles the text just the same.
2. **The route applies it whatever it is sent** (`handleOCR`): words for such a page are dropped, the rest are
   stamped, and when nothing is left the document is not rewritten at all (no undo entry for a change that was not
   made). Which pages were left alone rides in `X-Nib-OCR` (ADR-072: a fact about the returned bytes is a header),
   capped at 64 with a count of the rest.
3. **The window asks first**, once, at `GET /api/ocr/pages`, and does not render or recognise those pages — the
   recognition pass is the slow part. One read of the document for every page: asking the page map page by page
   re-reads the whole file each time (measured on a 14-page, 10.7 MB OCR'd scan: 1.2 s a page, against 2.3 s for all
   of them here). If the answer cannot be had the window reads every page and rule 2 still holds.
4. **The user is told in their terms** — "2 pages already had a text layer and were left as they are", or that there
   was nothing to add.

## Consequences

- A second press of OCR costs a couple of seconds and changes nothing, where it cost a full recognition run and
  doubled the layer.
- A document that is part scan and part OCR'd gets its missing pages done in one press.
- A page with visible print and no invisible text is still read, as before: a scan with a typed header, a fax line
  or a stamped page number must stay OCR-able, so "has text" is deliberately not the test.

## Declared gaps

- **A layer cannot be REPLACED.** Changing the language or the quality of a page already OCR'd means Undo, which
  exists only for the session that ran it; for a saved file there is no way. Replacing means recognising nib's own
  stamped forms, removing them with their optional-content group and the structure `TagOCRLayer` built over them,
  and re-stamping — /pending 851 part 4, remainder. Until then a file stamped short before ADR-092 stays short,
  and ADR-091's reading is what covers it.
- **A page with a partial layer** (another tool's, covering half the page) is skipped whole.
- **The window's half is not driven by a test**: OCR in a real browser is minutes, so tier 3 does not run it. The
  route's half is what cannot be skipped, and it is tested.

## Guards

`TestASecondOCRDoesNotAddASecondTextLayer` (internal/server/ocr_test.go): the second pass adds nothing to a layered
page, still stamps an unlayered one, names the page in `X-Nib-OCR`, rewrites nothing when nothing is left, and
`/api/ocr/pages` answers by the same rule.
