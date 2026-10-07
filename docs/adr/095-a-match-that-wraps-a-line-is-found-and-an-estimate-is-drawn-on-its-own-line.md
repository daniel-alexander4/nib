# ADR-095 — a match that wraps a line is found by both readings, and an estimate is drawn on its own item's line

**Status:** accepted (2026-10-06). Extends ADR-090 (a match is boxed from its glyphs; an estimate is replaced only by
a box that holds its centre). Closes ADR-090's declared gap "a match that wraps across a line is still not found" and
establishes the cause of the seven words it left as "cause not verified". ADR-090's rule is unchanged.

## Context

Two things about a search-redaction were wrong, and both were in the direction a redaction cannot afford.

**A match that wraps a line was found by nothing.** *Redact text…* has two readings of a page: the estimate, from
pdf.js's text layer (`scanTextMatches`), and the exact one, from the page map (`matchesInMap`). Each searched a row at
a time. A name whose first word ends one line and whose second begins the next is on no row, so neither reading found
it: no box, no count, and "No matches found" over a name in plain sight. Driven end to end through `scanTextMatches`
on the v1.189.4 tree, with a map and without one, a wrapped phrase returned **zero marks**. ADR-090 declared it and
the readme called it rare; for the built-in patterns it is, and for a typed name or address it is the ordinary case.

**An estimate was drawn on the wrong line.** ADR-090 measured seven words whose box took a neighbouring glyph and
guessed at four of them. The harness now records where each box came from, and the facts are:

- **Four words (taking 4–10 glyphs): an estimate kept beside its exact box**, as guessed — and the reason is not the
  centre rule. `buildTextRows` puts a pdf.js item in a row when its baseline is within 6 points (or 0.6 of its height)
  of the row's first item's, and the estimate drew every match in the row at `row.y`, that first item's baseline. In
  5–8pt type set in two columns a few points out of step, one row holds two baselines. Read off the four pages: the
  word's own baseline was 4.5, 5.8, 5.9 and 6.0 points **below** `row.y`. So the estimate stood that much too high;
  its centre fell just above the exact box and it was kept, blacking out the line above. A fifth word had the same
  kept estimate and happened to reach no glyph centre.
- The same misplacement **under-covers where there is no map to replace it**: an 8pt word's estimate stopped 2.4
  points above its baseline. The bottom third of its letters would have stayed on the page.
- **Three words (taking one glyph): each box was the map's own, and two of the three were the instrument.** The
  harness reads boxes off the page, where their edges are whole pixels — up to 0.8pt wider than the mark the app
  holds and applies. Scored as applied, two take nothing. The third is real: 5pt type, the glyph after the word
  1.39pt wide, its centre 0.70pt past the word and so inside the box's 0.75pt pad (`MATCH_PAD_X`).

## Decision

1. **`wrappedMatches(pieces, patterns)` (web/detect.js) is the one door for a match across a line end**, and both
   readings call it. A *piece* is a stretch of a row's text; a row is one piece unless a gap wider than the font size
   (`WRAP_GAP`) splits it — a column's gutter, a table's cell — because text wraps from the end of its own column.
2. **Text carries on in the next row down that has a piece beginning left of this one's end, in every such piece.**
   Which of them is the same column is not known. A wrong guess costs a search of two short strings and can only add
   a box; choosing one and being wrong would lose the match.
3. **A line end is read three ways:** as a space; and where the line ends in a hyphen (`-`, U+00AD, U+2010, U+2011),
   as nothing (`123-45-` / `6789`) and as nothing with the hyphen dropped (`confi-` / `dential`).
4. **A wrapped match is one box on each line it is on**, each drawn exactly as a match on that line would be — glyph
   boundaries, a short OCR stamp's stretch, a fitted OCR word's ink and its `hold`, a right-to-left OCR word whole.
   A match with ink on only one line of a chain is that row's own and is not found twice.
5. **It applies to every pattern, the built-in four included.** SSN, phone and card accept a space or a hyphen between
   groups, so a number broken over a line is now found. The cost is that digits ending one line and beginning the
   next can read as one number. That over-covers, the boxes are reviewed before anything is applied, and the other
   choice leaves a wrapped phone number on the page.
6. **The estimate is drawn from the baselines of the items the match is in** (`rowBaselines`), never from `row.y`.
   A match that runs over two items on two baselines reaches both. `buildTextRows` itself is untouched: the field
   detectors read its rows too.
7. **`placeMatches` is unchanged.** The four words are fixed by putting the estimate where the word is, after which
   the exact box holds its centre. Loosening the rule — "replace when the exact box is near" — was the other way to
   the same numbers and is refused: an estimate half a line off is exactly as far from the line above's exact box as
   from its own, and the rule exists so that a match the map did not place is never dropped.

## Consequences

Measured with `build/accuracy.sh` on this machine, 19 documents, 43 pages, 187 words, before (v1.189.4) and after:

- Words not marked **0 → 0**; found words fully covered 100% → 100%.
- Words with an estimate kept beside their exact box **4 → 0** (and a fifth, kept a line above its word, is gone:
  boxes drawn off a word's row 26 → 25).
- Words taking a neighbouring glyph, boxes read as the page draws them (the count ADR-090 reported): **7 → 3**.
  Read as the app applies them: **1** — the 5pt word above. It is over-covered by one narrow glyph and is left: the
  pad is what keeps a glyph's overhanging ink inside the box, and nothing here measured how little would do.
- Mean overreach **0.37 → 0.33** glyph-widths.
- The harness now says where each box came from (`bySource`, and per word `drawn`, `taken`), by comparing the boxes
  on the page with the map's own — the product carries nothing for it.
- The toast counts boxes, so a wrapped match reads as two "match(es)".
- A search now reads each row's neighbours: at most `WRAP_LINES` (3) rows deep, every piece of the next row tried.
  Cost on a dense table is unmeasured.

## Declared gaps

- **A match that wraps a page** is not found. Nor is one over more than three lines.
- **No real document in the accuracy corpus has a wrapped match scored**: `wordsToFind` picks single words. The rule
  rests on `test/jsdom/wrappedmatch.test.mjs`, which makes its own pdf.js page and map. Whether a real two-column
  page's gutter is wider than one font size is not measured.
- **Rows are still built from baselines within 6 points.** Two lines merged into one row are searched as one string
  with their items interleaved by x, so a phrase inside one of those lines that pdf.js split into several items can
  be missed by the estimate (the map reading groups by its own rows and is not affected). Not fixed here: the row
  builder is the field detectors' too.
- **The field detectors still place by `row.y`** (`findYesNo` and its siblings in web/detect.js). Same shape, not
  touched: another change owns that code.
- On one corpus page the estimate finds a word in places the map does not place it (26 boxes over a handful of
  words, all kept, as ADR-090 §4 says they must be). Why the map does not place them is not established.

## Guards

`test/jsdom/wrappedmatch.test.mjs` — twelve cases, `scanTextMatches` driven end to end from app.js's own text. Each of
36 conditions was weakened in turn and turned a named test red on an assertion: carrying on up the page, into any
piece, into every later row; a kept leading or trailing space; a line end read as nothing; each of the three hyphen
readings; a match on one line counted; a pattern not run from its start; two lines and four; a blank piece; a match
found twice; the map never splitting a row; `rowPieces`' gutter, joining space and order; either reading not
searching across lines; a part boxed from the wrong offset; the estimate's top or bottom taken from `row.y`.
Two more were removed rather than kept: a sort of the map's rows, already in order, which nothing could turn red;
and a per-row memo of `rowBaselines`, whose only red was a TypeError.

## Cost, measured at landing (2026-10-06)

`wrappedMatches` over synthetic table pages, five patterns, one reading (a page is read twice — the map and the
estimate): 60 lines × 1 stretch 5 ms; 60 × 6, 44 ms; 60 × 12, 170 ms; 120 × 12, 449 ms; 200 × 20 (4,000 cells on one
page) 3.2 s. It grows with the square of the stretches a line holds, because text is carried into every stretch of the
next line that starts left of this one's end. No real page was timed.
