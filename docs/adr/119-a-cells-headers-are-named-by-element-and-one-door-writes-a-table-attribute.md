# ADR-119 — a cell's headers are named by element, and one door writes a table attribute

**Status:** accepted
**Date:** 2026-10-09
**Context:** /pending 855 item 5 (the tree editor sets ColSpan, RowSpan and Headers) and /pending 854 item 1
(the tagger proposes tables). Both write the same four `/O /Table` attributes; before this the editor wrote
one (`/Scope`, through `withTableScope`) and nothing wrote the others. `internal/pdfops/structedit.go`,
`structwrite.go`, `structview.go`; `web/app.js`'s edit bar.
**Applies:** every writer of a table attribute or of an element's `/ID`.

## Decision

**1. One door writes a Table attribute: `withTableAttribute`** (ADR-009). Scope, ColSpan, RowSpan and Headers
all go through it, so each keeps the rule the scope edit had: the Table attribute object is copied, never
written through (an attribute object may be shared), and every other attribute object is kept as written.
`withTableScope` is gone. The table proposal (/pending 854) writes its cells' attributes through the same
door.

**2. A headers edit names ELEMENTS; nib writes the identifiers.** `/Headers` is an array of byte strings,
each of which must equal the `/ID` of a `TH` in the same table (ua1 7.5 t2). The edit carries the header
cells' object numbers, as every other edit names an element. A header cell that has an `/ID` keeps it and
`/Headers` holds that very string object; one that has none is given `nib-<object number>` (suffixed until no
element of the tree carries it). A header named must be another `TH` of the SAME table as the cell, or the
edit is refused — a header in another table would pass no checker and help no reader.

**3. A new identifier is entered in the `/IDTree`.** ISO 32000-1 Table 322 requires the tree once any element
has an identifier. veraPDF's PDF/UA-1 profile does not read it; nib writes it anyway. A document with none
gets a flat one, as an indirect object (pdfcpu's validator reads it only through a reference). A producer's
tree is added to and never rebuilt: the entry goes in byte order into the leaf whose `/Limits` hold it, and
every `/Limits` on the way down is widened.

**4. A span is written as given; an empty value removes it.** `1` is written as `/ColSpan 1`, not removed: a
class map (`/C`) can supply a span when `/A` has none, so removal and "1" are different statements. The
read is the checker's — the first Table attribute object whose value is an INTEGER, else 1.

**5. A cell edit does not judge the table.** A span that leaves a row short is written; the checker's 7.2
t41–t43 report it. Correcting a table often takes two edits, and refusing the first would make the second
impossible. What an edit still refuses is a tree that contradicts itself (`checkStructConsistency`).

## Consequences

- The view reads `colSpan`, `rowSpan` (1 where none is declared) and `headers` (element indices, as `kids`
  are) for every element; the edit bar offers spans on a `TH` or `TD` and the header cells of its own table
  as tick boxes. A header cell written inline has no object number and is not offered (/pending 855 item 3).
- `/Headers` naming an identifier no element carries is not shown and is dropped when the cell's headers
  are next set.
- **pdfcpu's relaxed validator rewrites a name-tree leaf's `/Limits` on every read**
  (`validateNameTreeDictLimitsEntry`), so a test that writes, reads back and inspects limits sees pdfcpu's
  repair, not nib's write. `TestAnIDTreeEntryWidensEveryLimitOnItsWayDown` reads the writer's own context.
- The page-subset carry still drops `/IDTree` whole (`structcarry.go`), as it did.
- **Covered by** `TestACellsSpansAndHeadersAreWrittenAndReadBack`,
  `TestACellEditThatDoesNotDescribeTheTableIsRefused`,
  `TestAHeaderCellKeepsTheIdentifierItHasAndANewOneCollidesWithNothing`,
  `TestANewIdentifierEntersTheIDTreeTheDocumentHas`, `TestAnIDTreeEntryWidensEveryLimitOnItsWayDown`,
  `TestATableCorrectedByCellEditsIsJudgedAsVeraPDFJudgesIt` (nib and veraPDF, six table clauses, before and
  after each edit), `test/jsdom/tagedit.test.mjs` and tier 3's `tagcorrect` (header cells named by keyboard
  alone; 7.5 t1 passes with no scope).
