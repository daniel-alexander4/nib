# ADR-188 — Plain text and CSV are laid out by Nib's own renderer, and the caps are measured

**Status:** accepted
**Date:** 2026-11-02
**Context:** `PLAN-import-export.md` P01.S03b (D10, D14, W17, Y6); extends ADR-185, ADR-187.

## Decision

- **`.txt` and `.csv` need no outside tool.** `textImport` (importtext.go) writes the Markdown that says what the file says and `mdpdf` lays it out with Nib's faces, tagging and font honesty. Text is a fenced block whose fence outruns the longest backtick run; a CSV is GFM tables of at most `csvRowsPerTable` rows (mdpdf refuses a longer table), the header repeated on each. The delimiter (comma, semicolon, tab) is sniffed from the first line outside quotes. Text is read as UTF-8, UTF-16 by its byte-order mark, else Windows-1252.
- **A cell is literal.** Every character the inline syntax could read is backslash-escaped, so `*x*` in a cell prints `*x*`. This needed `mdpdf` to resolve CommonMark backslash escapes at all: it printed the backslash (`\*`) for any Markdown file. The fix is in `mdpdf`'s text case, outside code spans, with its own red-proved test.
- **Caps are taken from the source before layout and refuse by name:** 40,000 text lines, 10,000 CSV rows, 24 columns (mdpdf's), 1,000,000 cells, then the 1,000-page cap on the result. Placeholders and an image directory do not apply to these two (a file's `{{x}}` is its text).

## Measured (this machine, other sessions running; one run each)

CSV, 6 columns: 1,000 rows 2.4 s / 36 pages; 4,000 rows 13 s; 16,000 rows 83 s / 572 pages; 32,000 rows 279 s (past the page cap); 50,000 rows refused by pdfcpu's own 64 MiB JSON limit. Text: 10,000 lines 3.2 s; 40,000 lines 10.5 s / 834 pages. At the caps: 10,000-row CSV 65 s, peak RSS 345 MB; 40,000-line text 10.5 s. CSV cost is super-linear, which is why the row cap sits well under the page cap's ~28,000 rows.

## Declared limits

A CSV past 24 columns, which LibreOffice used to convert, is now refused (`ErrCSVTooWide`). There is no separate wall-clock cap on these two producers: the row and line caps bound the work, and the 65 s worst case is under the 2-minute budget the raster producers use. CSV cells are never formulas here (output is a PDF).
