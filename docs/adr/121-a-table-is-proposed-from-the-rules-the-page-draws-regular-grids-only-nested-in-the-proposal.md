# ADR-121 — a table is proposed from the rules the page draws, regular grids only, nested in the proposal

**Status:** accepted
**Date:** 2026-10-09
**Context:** /pending 854 item 1. The tagger proposed a FLAT list of headings, paragraphs and list items from
text alone, so a table's cells were proposed as paragraphs in whatever order their baselines gave and a tagged
table could only be made by hand in another tool. `internal/pdfops/tablegrid.go`, `grouping.go`
(`readPageTableLayout`), `proposer.go`, `tagreview.go`, `tagcommit.go`; `/api/tags/propose`, `nib tag propose`,
the Tags review card in `web/app.js`.
**Applies:** everything that proposes, reviews or commits inferred structure.

## Decision

**1. A table is read from what the page DRAWS.** A table is a regular ruled grid: horizontal and vertical rules
bounding at least two rows and two columns, every side of every cell drawn (`ruledGrids`). A straight stroke is
a rule; so is a thin filled rectangle, and a stroked box is its four sides, so a grid drawn as tiled rectangles
reads as the rules those share. Text with no rules is never a table — columns of figures, a two-column page and
a price list all line up. The rules come from the page map's reader (`readPageShapes`, ADR-088); the tagger sets
no `keepShapes` on a walk of its own.

**2. Regular grids only.** A complete frame of at least two rows and two columns with a side missing inside it
is how a merged cell is drawn. It is NOT proposed as a table: its text is proposed as paragraphs, as before, and
the page says a ruled table with merged cells was not read as one. No ColSpan or RowSpan is proposed. Also not
read, each said on its page: grids that overlap, and a grid holding rotated text. Not read and not reported: a
grid that holds no text, one whose rules run more than 3pt past its outermost rule the other way (a column with
no side), and a page that rules past `gridMaxCrossings`.

**3. A run belongs to the cell that holds the centre of its box**, decided run by run before any line is made.
A cell's lines are one element. What is left of the page goes through `groupRuns` unchanged — one grouping, over
fewer runs — except that a paragraph with one line above a table and the next below it is ended at the table. A
table's text still counts toward the document's body size.

**4. The proposal is nested.** An element names its `parent` (-1 at the top level). A table is `Table` → a `TR`
for each row → a `TH` or `TD` for each cell, consecutive and row by row; the first row's cells are proposed as
headers. A cell with no text is still an element. A Table and a TR carry no content; their text is their cells',
joined, so a table that changed is stale. The table sits where its top-left cell would be read.

**5. A table is reviewed whole.** A cell may be made `TH` or `TD` and nothing else; a Table and a TR keep their
type; no other element may be given a table's role. Every row and cell stays directly after the element proposed
before it — which holds the parent first, the subtree unbroken and the order — while the table as a whole moves
like any element. Ignoring the Table ignores everything under it; a row or a cell is not ignored by itself. Each
is `ErrTagsReview` with its own sentence. The review's wire shape is unchanged.

**5a. A table that is not one is reviewed as `P`, and its cells are written as paragraphs.** A ruled form tiles
as regularly as a table (measured on the local corpus: a tax form's boxes, a membership form's ruled lines), and
ignoring is no answer to it — an ignored element's text becomes decoration. The Table reviewed as `P` writes each
cell that has text as a paragraph, row by row, where the table stood; the Table, its rows and its empty cells are
not written. Ticked as ignored too, it is ignored.

**6. The commit writes scope through the one door.** Table and TR are grouping elements; a cell is the element
its text is bracketed under, or a grouping element when it has none. A `TH` gets `/Scope` through
`withTableAttribute` (ADR-119): Column in the first row, else Row in the first column, else Column. The rules
themselves become artifacts with every other uncovered drawing.

## Consequences

- Detection is a function of the bytes alone, which `CommitTags` relies on: it proposes again and requires the
  review to match.
- The proposer reads each page with the map reader's walk (glyphs, marks and shapes kept) where it read runs
  only. On the local corpus — 51 documents, 505 pages — the elements proposed on every page with no table are
  identical before and after.
- **LibreOffice paints nothing for a table with no borders**, so `tableAndFigureODT`'s table is not proposed;
  the producer test rules its cells.
- **Declared gaps:** merged cells; a table ruled only between rows, or with no outer sides; a table with no
  rules at all; a header column (a reviewer retypes the cells); a table continued on the next page is two
  tables; a ruled form whose boxes happen to tile regularly is proposed as a table, and a reviewer says it is not one
  (5a) — the paragraphs it then gets are one per box, row by row, not a re-grouping of the page.
- **Covered by** `TestARuledGridIsProposedAsATable`, `TestWhatIsNotARegularRuledGridIsNotATable`,
  `TestOverlappingGridsAreReportedNotRead`, `TestAPageOfRulesBeyondTheBoundIsProposedWithoutTables`,
  `TestATableSitsWhereItsTopLeftCellWould`, `TestATablesTextStillCountsTowardTheBodySize`,
  `TestACommittedTableReadsBackNested`, `TestATableReviewKeepsTheTableWhole`,
  `TestATableReviewedAsNotOneIsWrittenAsParagraphs`,
  `TestAProducersRuledTableIsProposedAsTheProducerTaggedIt` (LibreOffice's own table against its own tags, and
  veraPDF: no table clause, and no clause the same document committed without tables does not fail),
  `TestAHandBuiltTableFailsNoTableClause`, `TestATableWithNoRulesIsNotProposed`,
  `TestTheReviewDoorsRouteThroughTheProposer`, `TestTagProposeIndentsATablesRowsAndCells` and
  `test/jsdom/tagsreview.test.mjs`.
