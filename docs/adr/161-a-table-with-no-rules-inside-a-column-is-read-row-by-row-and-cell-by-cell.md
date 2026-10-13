# ADR-161 — a table with no rules inside a column is read row by row and cell by cell

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-155 decision 4 (a run does not continue a line across a gutter), and closes two of the limits
that ADR declared. ADR-121 is unchanged: only a RULED grid is proposed as a table.
**Context:** `/pending 871`. After ADR-155 the two cells of a row were separate lines, but where such a table sits
inside one page column the paragraph step still wove the two cell columns together (irs-fw9 p4): a "paragraph"
took a line from the left cell, then one from the right. The table has no drawn grid (`ruledGrids` returns 0), so
the tag proposer never saw a table and reflow, which has no gate on that state, could edit a woven paragraph.
And a contents list whose page numbers sit past the join gap had every number parted from its title.
**Applies:** `internal/pdfops/grouping.go` (`cellRowsOf`, `cellRows.paragraphs`, `columnParagraphs`,
`endsInLeader`).

## Decision

**1. A stretch of three or more rows set side by side across one gap is read row by row, and within a row cell
by cell**, down to the first line drawn across the gap. A cell's "stopped short" edge is where the next cell
begins; a paragraph ends where the stretch ends. Reflow and the tag proposer read it through the one grouping
door. The page's column count and its "side by side" note are unchanged.

**2. The cells are NOT read as whole columns.** Measured and refused: 18,702 corpus paragraphs fell to 10,898, a
column of figures becoming one paragraph.

**3. A line that ends in a leader — four or more trailing dots, spaced or not — continues across a gutter to its
page number** (`endsInLeader`). An ellipsis is three and is not a leader.

## Consequences

Producers corpus: paragraphs holding two adjacent lines clear of each other 285 → 23; reflow's mixed-state
refusals 44 → 3; multi-line paragraphs in the reflow census 2,436 → 2,268 with "not in place" unchanged at 65;
1,171 paragraphs re-set with every other line unchanged, 0 moved. Gutter cuts 186 → 182, the four gone being the
four contents lines of irs-p15 p8. Eleven and four mutations red respectively; three conditions that would not
go red were removed as inert.

**Declared:** two cells whose second lines share a baseline read as two rows (parted, not woven); prose
sub-columns under a spanning heading still read across; where three lines share a baseline the gap taken changes
the order on irs-f1040 p1, irs-fw9 p1 and fda-170357 p42, none clearly better; three rows is the floor.
