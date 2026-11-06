# ADR-211 — A table that crosses pages is one Table, and its repeated header is an artifact

Status: accepted, 2026-11-05 (/pending 951, v2.49.7). Supersedes ADR-210 decision 2 only; ADR-210 decisions 1, 3 and 4 stand.

1. **The tagger's table state belongs to the document, not the page** (`tableState`, `newTableState`, created once in `tagFromRolesAndFigures` and
   passed to `tagOnePage`). `tagOnePage` kept `tableRef`/`curTable` as locals, so a 200-row table was 8 `Table` elements (measured), one per page. A table
   that crosses a page is now ONE `Table`; a block that is not a table cell closes it, a new table ordinal starts another, and a row on a later page is
   a `TR` under the same `Table` with its own `/Pg`. A test that calls `tagOnePage` alone passes `nil` and gets a fresh state.
2. **The header drawn again on a continuation page is `Role.Repeat`, and the tagger brackets it as `/Artifact`** and writes no element: the header
   exists once in the tree, on its first page. This is what ADR-210 said it would become when the tables were joined; ADR-210 repeated the header as
   `TH` precisely because each page's `Table` stood alone. The repeat does not touch the table state, so the rows after it stay in the same table.
3. **Verified** against veraPDF ua1 (a 200-row table: only `5 t1`, which Nib refuses to write), against the tree (1 `Table`, 201 `TR`, 2 `TH`,
   400 `TD`), and red against each half: state made per page again (8 tables), the tagger ignoring `Repeat` (a second `TH` set), the layout not setting it.
4. **Declared:** a table with a picture in a cell on a continuation page keeps the picture's Figure under its own page's cell (ADR-210 decision 4),
   so one `Table` holds cells on several pages; no other consumer of `Role` reads `Repeat`.
