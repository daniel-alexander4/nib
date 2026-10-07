# ADR-098 — print is boxed by the ink its font's own glyphs reach, and only an embedded program says what that is

**Status:** accepted (2026-10-07). Extends ADR-093 (a fitted OCR word is boxed by its ink) to print, and ADR-088
(the page map is the one door). Supersedes nothing: `Rect`, `runBox` and its two constants are untouched, a fitted
OCR word is read as ADR-093 says, and a run whose font gives nothing is boxed exactly as before.

## Context

A search-redaction's box is as tall as its runs' `Rect`, and `Rect` is `runBox`'s: a full size above the baseline and
a quarter below, whatever the letters are. That is a line's reach — right for reflow and for grouping runs into
lines — and it is not a word's ink. /pending 851 recorded two consequences for print and no measurement of either
(parts 1 and 2): the box reaches the line above, and a descender deeper than a quarter of a size is left showing.

**There was nothing to measure with.** `build/accuracy.sh` scored a box ACROSS the page against the map's glyph
cuts, which come from the font's own widths. Up and down the map has only the constant, so scoring a box's height
against the map would score the constant against itself. The instrument was built first: a vertical-reach column
(`test/accuracy/reach.mjs`) whose truth is the page's pixels, drawn by poppler at 300 dpi — neither Nib nor the
browser Nib runs in.

**Measured before any change** (19 documents, 43 pages, every word of three letters or more the map can box):

- 6,490 words scored, 42 not (type over a watermark, a line through the word). **774 boxes reach the ink of the line
  above** (11.9%; median 1.1pt, ninetieth percentile 1.8pt), 55 the line below. The word's own ink outside its box:
  **0**. Of the 186 words the harness searches in the app: 21 above, 1 below, 0 outside.
- The corpus's own ink stops at 0.82 of a size above the baseline (median 0.72) and 0.24 below. It has no Arabic
  and no accented capital, so it cannot say what part 2 costs; the box's 0.25 below is 0.01 clear of its deepest tail.

**Three remedies were candidates, and two were refused by what the corpus's fonts say.**

- *A tighter constant.* 0.85 of a size holds every corpus word and takes 774 down to 55. It is a fact about
  nineteen English documents: a capital with a ring or an acute stands at 0.9 to 1.0 in the same faces. A constant
  measured on a corpus without them is ink left outside a redaction on the first document that has one.
- *The font descriptor.* `/Descent` is **positive** in some thirty of the corpus's descriptors, one `/CapHeight` is
  1170, one `/FontBBox` runs 205..771 up the page, and `/Ascent` is anything from 612 to 1077 for faces whose
  letters all stop near 730. A descriptor is a statement by whoever wrote the file.
- *The embedded program's own outlines.* The only thing that bounds what a glyph paints. Read from the points, not
  from the box each glyph declares for itself (a renderer does not read that box either).

## Decision

1. **A run of print carries its own ink on the map where its font's embedded program says what that is**
   (`MapText.Ink`, `pdfops.printInk`, `internal/pdfops/fontink.go`) — the same field, and the same meaning, as
   ADR-093's: what a redaction of the run covers, beside a `Rect` that stays the line's reach.
2. **What is read is a TrueType program (`/FontFile2`), from its points** (`readGlyfInk`): every glyph's lowest and
   highest point, a part of a compound glyph carried by its move and its scale up the page. A glyph whose part is
   placed by matching points or by a full two-by-two is not read, **and then the font is not** — nor is one cut
   short, self-referring or past `fontInkLimit` (32 MiB decoded). A font is vouched for whole or not at all.
3. **One program gives one of two answers, and they are not equally good.**
   - *Codes that ARE glyph numbers* — a Type0 font under `Identity-H` whose CIDFontType2 has no `/CIDToGIDMap` or
     `/Identity` — give each run the ink of the glyphs it shows. It may be deeper or taller than the reach; then the
     box grows to it. This is how current writers set everything outside Latin-1, Arabic included (part 2).
   - *Any other TrueType font* gives the reach of EVERY glyph in its program, because which glyph a code draws is
     decided by the program's own tables, and a reader that maps them differently from the renderer has the wrong
     glyph's ink. A subset holds little but the glyphs the document shows, so this is nearly as tight. **It only
     ever tightens**: clamped to the line's reach, so a full font whose tallest glyph stands past a size changes
     nothing.
4. **No ink, and so the box as it was**, for: a font that is not embedded; Type 1 and CFF programs; Type 3; a run
   that is stroked (its ink is wider than its outline by half the line), turned, hidden, or on a turned page.
5. **The reader draws a print box to the ink and a whole point clear** (`INK_PAD_Y`, web/detect.js), not the half
   point every other box gets: the ink is an outline, a renderer sets it on its own pixel grid (measured at 300 dpi:
   ink up to 0.36pt past the outline), and the quarter of a size of slack the reach had is gone. **And never past the
   box the reach drew**, on a side where the ink is inside the reach — otherwise glyphs that stop a hair short of
   the reach would make the box larger than before.
6. **`hold` is ADR-093's and unchanged in meaning**: a box drawn to ink carries the box its reach would have drawn,
   and `placeMatches` tests an estimate's centre against that. For print it is exactly the box every match had
   before, so which estimates are replaced does not move.
7. **`build/accuracy.sh` gains the vertical-reach column**, and a change to how tall a box is drawn is judged by it:
   `inkOut` must be 0 before and after, and no page's reach may rise.

## Consequences

Same corpus, every word, before → after (offline over the prepared maps; the harness's own run is in
`memory/instruments/page-map-accuracy.md`):

- A box reaching the line above: **774 → 399** of 6,490. Reaching the line below: 55 → 55. The word's own ink outside
  its box: **0 → 0**; the least room left over any word's ink is 0.64pt above and 0.70pt below. No page worse on
  any of the three. 3,036 words have a different box.
- In the app (the harness's own run, 43 pages; the document that opens in signing mode is not driven): the 186
  searched words reach the line above 21 → 14 and below 1 → 1, ink outside 0 → 0. Every other column is unmoved —
  187 words, none missed, all fully covered, overreach 0.326 glyph-widths, 1 takes a neighbour, all boxes from the
  map; detection found 98.5%, well placed 98.2%.
- The 399 left are runs with no ink: 27% of the corpus's print runs are in an embedded TrueType program (1,749 of
  6,514); the rest are Type 1C (both IRS forms) or not embedded.
- Cost, measured: all 132 TrueType programs of the corpus read in 25 ms together, the slowest 2.6 ms (779 KB). Read
  once per font per map, and only when the map is asked — reflow and tagging never ask.
- The fitted-OCR path is untouched: the reader's gate was `hidden`, and is now "the map carries ink".

## Declared gaps

- **CFF and Type 1 programs** — half the corpus's reaching boxes. Their outlines are charstring programs; a second
  reader, and `internal/uacheck`'s reads widths, not points.
- **A simple TrueType font in a script with deep tails** keeps the reach below (answer two only tightens). Unmeasured:
  the corpus has no such font, and no Arabic print at all — part 2 rests on `TestWhichInkARunIsGiven`.
- **The truth is poppler's pixels, and a redaction is applied to pdf.js's.** The two set the same outline on
  different grids; the whole point of pad is the allowance, and it is not measured against pdf.js.
- **The column guesses once**: where two lines are set closer than an i is to its dot, it stops a word's ink where
  the next line's letters are likely to begin (0.8 of ITS size above ITS baseline, 0.3 below). 42 words are not
  scored at all.
- A font that draws outside its own outlines by hinting instructions is not modelled; neither was it by the constant.

## Guards

- `TestAProgramsGlyphsReachWhereAnotherReaderSaysTheyDo` — every glyph of nib's vendored faces against x/image's
  bounds, which share no code with the reader.
- `TestAnAuthoredPagesPrintCarriesTheInkOfTheGlyphsItShows` — the whole path, the oracle reaching glyphs through the
  face's character map and not the page's codes; `TestPrintInAFontThatIsNotEmbeddedHasNoInk` is its other half.
- `TestWhichInkARunIsGiven`, `TestWhichFontsCodesAreGlyphNumbers`, `TestAProgramThatCannotBeReadWholeGivesNothing`.
- `TestTheReachColumnReadsTheBaselineRunBoxWrote` — the column reads a baseline out of `Rect`, so it carries
  `runBox`'s constants; changed in one place only, it fails.
- `test/jsdom/placematches.test.mjs` — "print is boxed by its font's own ink…": the pad, both caps, `hold`, growth
  past the reach, a match over a run with ink and one without.
- 34 mutations, one condition each, all red for an assertion.
