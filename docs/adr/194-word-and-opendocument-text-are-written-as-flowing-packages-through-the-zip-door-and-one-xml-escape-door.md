# ADR-194 — Word and OpenDocument text are written as flowing packages through the zip door and one XML escape door

Status: accepted, 2026-11-03 (PLAN-import-export P03.S01, v2.38.0). Extends ADR-193, ADR-187, ADR-192; D21, D22, R1, R8, X7, Y3, Y9.

**Flowing, one form per format (D21).** `internal/docwrite/docx` and `internal/docwrite/odt` write the document model as
text that reflows: Word's own styles (Heading 1-6, List Paragraph, Table Grid) with real `numbering.xml` lists, ODF
`text:h`, `text:list` and `table:table` with real covers. No positioned frames. Both import `docmodel`, `ziputil` and
`docwrite/xmlw`, and no PDF reader; `TestNoWriterReachesAPDFReader` reaches them through `docwrite/...` without a change.

**One XML door.** `xmlw.Esc` is `docmodel.Clean` then the five entities, and every text and attribute value either writer
emits passes it. A tab and a line break are elements, never characters (`w:tab`, `w:br`, `text:tab`, `text:line-break`),
and an ODF run of spaces is `text:s`. `xmlw.SafeLink` re-asks the link allowlist the model already applied, because a
hand-built model is hostile in the tests. Part names and picture names are fixed (`word/media/img-N.ext`,
`Pictures/img-N.ext`); the title is the name the user confirmed (`dc:title`), the language is a BCP 47 shape or `und`.

**The zip door.** Both packages are built by `ziputil.Write`: the ODF `mimetype` is first, stored, at byte 38; no
`archive/zip` caller was added, so the R8 census is unchanged. The package holds no creator, last-modified-by, timestamp,
revision id, thumbnail or conformance-claim key (`TestDocxHoldsOnlyItsNamedPartsAndNoClaims`,
`TestODTHoldsOnlyItsNamedPartsAndNoClaims`: every part name against an allowlist).

**Table spans** are laid on the grid once (`xmlw.Grid`): a row-spanning cell leaves a covered position (`w:vMerge`,
`table:covered-table-cell`) in each row below, a column span is `w:gridSpan` / `number-columns-spanned` plus ODF's covered
cells, and a short row is padded to the grid.

**Judged** by parsing the written XML (`w:t` runs joined per paragraph with no separator, as Word joins them;
`office:text` in `content.xml`) against the model's words in order, plus heading, list, table, link and picture counts,
over the model whose every value tries to be markup; and, where `soffice` is installed, a confirmatory LibreOffice run
that converts each package to text and requires the model's words in order (`wtest.ReadBack`; the rendered list labels
and the placeholder line are the nine extra words the test pins). Without `soffice` the confirmatory test skips and
says "NOT RUN"; the writer is never the thing that is skipped.

**The route and the window.** `/api/export/document` answers `format=docx|odt` with the packages' media types and the
same `X-Nib-Export` facts; the writers return `([]byte, error)` and the three older ones are lifted by `plain`. A model
with no block is nil, so the route's `empty` 422 holds. *Document as…* lists Word (.docx) and OpenDocument text (.odt)
after the three it had, each with its `EXPORT_KEEPS` sentence saying the layout is not kept (L6).

**Declared:** a picture's alt text is the `descr` / `svg:desc` only; a picture with no bytes is an italic line naming it;
headings are not numbered; no page size or header/footer from the PDF (the page is US Letter); RTL is the paragraph
`bidi` / `writing-mode` only, run-level mixed direction is Word's and LibreOffice's own; Word itself is not run (no
Word here); `epubcheck` and EPUB are S03.
