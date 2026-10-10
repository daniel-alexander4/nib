# ADR-123 — a ruled grid is a table only with a quarter of its cells filled

**Status:** accepted
**Date:** 2026-10-09
**Supersedes:** ADR-121 §2 in one clause only — "not read and not reported: a grid that holds no text". The
rest of ADR-121 stands.
**Context:** /pending 859. ADR-121 proposes a table wherever a page draws a regular ruled grid that holds any
text at all. Nobody had looked at what that proposes on real documents. `internal/pdfops/grouping.go`
(`groupRunsAndTables`).
**Applies:** the table proposal.

## The measurement

The local corpus — `~/nib/producers` and the documents listed in `~/nib/accuracy-corpus.txt`, two shared — has
31 proposed tables. Read from each table's size, how many of its cells hold text, and its first row (a probe's
print-out; the pages were not looked at by eye):

- about 23 are tables — every one under `~/nib/producers`, three of them label-and-value grids whose headers
  run down the first column;
- 5 are a form's boxes that happen to tile: two number-box grids of 2×2 and 3×2, two 4×2 cover panels of about
  90 characters a cell, one 4×2 signature block of about 160;
- 3 are near-empty worksheets: 4 cells of 310 filled, 3 of 20, 7 of 72.

The emptiest of the tables is 9 cells of 24.

## Decision

**1. A grid is proposed as a table only when at least a quarter of its cells hold text** (`tableFillDivisor`).
A cell holds text when a run that joins a line has the centre of its box in it; two runs in one cell fill one
cell. Exactly a quarter is a table.

**2. A grid with fewer says nothing on its page.** Its text is proposed exactly as it would be with nothing
drawn, as an empty grid's always was — an empty grid is the same rule's first case, not a second one. It is not
an `unsupported` note: nothing on the page went unread.

**3. The shape rules are unchanged.** No rule separates the five form-box grids from the label-and-value
tables — same sizes, same fill, and the amount of text in a cell overlaps. They stay with the reviewer's *Not a
table* (ADR-121 §5a).

## Consequences

- On the corpus the proposal drops from 31 tables to 28: exactly the three worksheets. Every other table, and
  every element proposed outside one, is as before.
- **Declared gap:** a ruled sheet with a row of headings and blank rows under it — a sign-in sheet, a log to
  fill in — is fewer than a quarter filled once it has more than three blank rows to each row of headings, and
  is proposed as paragraphs. The 7-of-72 grid is one: its first row reads as headings. A reviewer cannot make it
  a table in the review card; the tree editor can, once it creates tags (/pending 855).
- Detection stays a function of the bytes alone.
- **Covered by** `TestAGridWithFewerThanAQuarterOfItsCellsFilledIsNotATable` (each side of the quarter, and two
  words in one cell) and `TestWhatIsNotARegularRuledGridIsNotATable`'s empty grids.
