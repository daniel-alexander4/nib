# ADR-196 — a table meets one bound in the model, and no writer is trusted to have its own

Status: accepted, 2026-11-03 (PLAN-import-export P03 phase close, v2.39.1). Extends ADR-009, ADR-192, ADR-194, ADR-195; Y9.

**The defect.** A cell's `/ColSpan` and `/RowSpan` come from the PDF's tag tree and were passed through
(`tableSpan` returns the integer as written). Markdown padded a row to the span (`ColSpan` 2,000,000,000 is a memory
exhaustion in the user's own process), and DOCX/ODT clamped one cell at 256 but padded every row to the widest, so 100
cells at span 256 above 10,000 one-cell rows was about 256 million written cells.

**The door.** `docmodel.BoundTable` is the one place a table meets a bound, called from `read.Read`'s page loop
(`boundTables`) for the tree's tables and the proposal's alike: spans clamp to `MaxSpan` (256) before they are summed
(the sum overflows otherwise), and a table wider than `MaxTableColumns` (63, Word's own limit) or past `MaxTableCells`
(50,000 = rows × widest row) is written as ONE PARAGRAPH PER ROW, cells joined by ` | `, every word kept. The count is
`Report.TablesAsText`, rides in `X-Nib-Export` as `tablesAsText`, and the window says it. The writers keep their own
clamp as a second line for a hand-built model; they are not the bound.

**Shared rules got one home.** `html` and `markdown` had their own link allowlist, language test, numbered-marker test,
title fallback and picture MIME map; they now call `xmlw`, so a malformed language is `und` in every format
(ADR-009). `html.Write` is nil for a model that renders nothing, so the route's `empty` 422 holds for every format.

**Grid.** A cell whose columns run over a column a row span from above still holds ends that hold: the cell that began
it keeps the rows it covered (`RowSpan` and `Restart` follow), and no stray cover is written under a later row.

**Pictures.** A written picture's longest side is 22 inches (`xmlw.MaxPx`), Word's extent limit; the earlier clamp was
20,000 px, 208 inches.

**Declared:** the output of an export is not capped in bytes (only upstream: 512 MiB document, 32/256 MiB of extracted
pictures), and the route has no concurrency gate — a measured peak is /pending work; list depth in `html`, `text` and
`markdown` trusts the reader's 200; Markdown writes a first item at level 2 as an indented code block; `epubcheck` is
still not installed here (`wtest.EpubCheck` runs it where it is and skips by name where it is not).
