# ADR-090 — a search match is boxed from its own glyphs, and an estimate is replaced only by a box that holds its centre

**Status:** accepted (2026-10-06). Step five of ADR-088's five. Extends ADR-088; supersedes nothing.

## Context

*Redact text…* finds matches in pdf.js's text layer and drew each box from an estimate: the characters of a text item
measured in `16px sans-serif`, rescaled to the item's width, the box then padded by 0.8 of a line height each side
because the estimate could not be trusted to reach the word's edges. Measured (ADR-088): every found word was covered,
the box ran a mean 2.6 glyph-widths past it, and in 72% of cases it blacked out neighbouring glyphs nobody asked for.

For a redaction that is not a cosmetic fault. The page is flattened under the box, so a box two glyphs too wide
destroys two glyphs of the next word — on an executed document, of a party's name or a figure.

The page map (ADR-088) has each glyph's boundary from the font's own widths.

## Decision

1. **`matchesInMap(map, patterns)` runs the same patterns over the page's text as the map has it** and boxes each match
   from its first glyph's left boundary to its last glyph's right, plus 0.75pt across and 0.5pt up and down. Runs on
   one line are joined in reading order; a gap wider than 0.15 of the font size is a space, so a pattern's `\b` holds
   where a label and a number are set as separate runs, and a number set in two abutting runs is one match.
2. **`placeMatches(estimated, exact)` decides which box a match gets**, per page:
   - an estimated box is replaced only by an exact box that **holds its centre**;
   - otherwise it is **kept**, whatever exact boxes it overlaps;
   - an exact box that replaced nothing is a match the estimate did not find, and is added.
3. **Why the centre, and not overlap.** The estimate over-covers evenly about its match, so its centre is the match's.
   Its edges reach into the neighbouring words — that is the defect — so "an exact box overlaps it" is true of a
   neighbour's box as well as its own. A match the map did not place, standing beside one it did, would then be dropped
   as already covered, and it would not be. The replacement rule can make a box tighter; it cannot remove one.
4. **What the map does not place keeps its estimate:** a page the server cannot map, a run without glyph boundaries
   (turned text, a turned page), and **hidden text** — an OCR layer's glyphs say where the invisible words were
   stamped, not where the scanned ink is (step six).
5. **The message says how many boxes were placed by estimate**, because the two kinds are not equally tight and the
   user reviews them before applying.

## Consequences

Measured (`build/accuracy.sh`, 19 documents, 43 pages, 189 words):

- Mean overreach **2.6 → 0.77** glyph-widths; boxes taking a neighbouring glyph **72% → 3.5%**; found words fully
  covered 100% → 100%; words not marked 14.8% → 9.5%.
- The 9.5% was mostly the instrument. A word in the same place on two pages now draws the same box twice, and the
  harness told a new box from an old one by value; a word that also occurs inside a longer word on its line
  ("Speech", "ashlandspeech") was scored as one box across both. With both corrected (187 words): **not marked 0%**,
  overreach **0.37** glyph-widths, a neighbouring glyph taken in **3.7%**.
- The 3.7% is seven words. Three take one neighbouring glyph, with under half a glyph-width of overreach. Four take
  four to ten and run about one glyph-width past the word each side, which is the estimate's shape: the likeliest
  reading is an estimate kept *beside* its exact box because its centre fell outside it. That cause is **not
  verified** — the harness records the union of a word's boxes, not which kind each was. All seven are over-covered,
  none under-covered.
- A search asks the server for each page's map: measured at a median 0.8 ms a page, 134 ms at worst (ADR-088).

**Declared gaps.** A match that wraps across a line is still not found (unchanged). A scan's matches are estimates
until step six. The box's height is the run's text box, 1.25 of the font size.

Guards: `test/jsdom/placematches.test.mjs` — nine cases; sixteen conditions each weakened in turn, each turning a
named test red, among them "replace on any overlap" and "replace ignoring the line".
