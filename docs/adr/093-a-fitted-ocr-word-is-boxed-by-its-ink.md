# ADR-093 — a fitted OCR word is boxed by its ink, and part of a right-to-left OCR word takes the whole word

**Status:** accepted (2026-10-06). Extends ADR-092 (the stamp is fitted to its scanned box) and ADR-090 (a match is
boxed from its glyphs). Supersedes nothing: a short stamp is still read as ADR-091 says, and print is untouched.

## Context

Two readings of a fitted OCR layer were wrong, both in what a search-redaction covers (/pending 851, parts 1-3).

**Up and down.** The page map gave every run the box reflow uses for a line's reach: a full size above the baseline
and a quarter below (`runBox`). ADR-092 set a fitted word at its true em — a median 1.32 of its ink's height — so
that box stood 1.65 times as tall as the word. On two real scans a search-redaction box reached another line's ink
for 13 of 40 and 5 of 39 words. The same box stopped a quarter em down, short of an Arabic descender (1.7pt of ink
below it on a synthetic sample).

**Across, for right-to-left scripts.** The stamp is set glyph after glyph, unshaped, in reading order — so a Hebrew or
Arabic word's first letter is the LEFTMOST glyph of its stamp, and the rightmost letter of the scanned word. A match
on part of such a word was boxed from the stamp's glyph boundaries, i.e. at the mirrored end: the letters asked for
were left showing and the others covered.

## Decision

1. **The map carries a fitted OCR word's own ink beside its box** (`MapText.Ink`, `pdfops.fittedInk`): [top, bottom]
   up the page, what `wordInk` measures for that text in that face — which is the scanned word's box, because that is
   what the fit put the ink on. Only for a hidden run inside a form, not `Short`, upright on an unturned page (where
   `Cuts` are), in a face nib stamps an OCR layer in (`ocrFontFiles` and Roboto); the face name is checked against
   that table before the face is loaded, because `ocrFace` remembers every name it is asked and a document chooses
   its own font names.
2. **`Rect` does not change, for any run.** It stays the line's reach, and three things read it as that: reflow
   (`runBox` and its constants are untouched), the reader's grouping of runs into lines, and rule 4.
3. **A search-redaction over a fitted word is drawn to its ink**, plus the usual half point (`matchesInMap`). Print, a
   short stamp (ADR-091 reads its box out and pads it) and another tool's hidden text are drawn to their reach, as
   before.
4. **An estimate is still matched against the reach.** A box drawn to ink carries `hold`, the box it would have had,
   and `placeMatches` tests an estimate's centre against that. pdf.js sets a fitted word's span about four points
   above its ink, so the estimate's centre is inside the reach and — for a word with a descender — above the ink:
   with the ink as the test, 6 of 39 searches on a real scan kept the estimate beside the exact box, a second box
   over the line above. ADR-090's rule (centre, never overlap) is unchanged; only which box it is asked of is named.
5. **A match that takes any part of a hidden run in a right-to-left script takes the whole run** (`RTL` in
   `web/detect.js`). Where the letters of a scanned word are is not known, so the reader does not pretend to know.
   Print in the same scripts is where the page set it and is boxed by its glyphs as before.

## Consequences

- Measured on the two scans ADR-092 was measured on (the layers it wrote; 40 and 39 whole-word searches in the app):
  a box reaching another line's ink 13 → 0 of 40 and 5 → 3 of 39; ink above, below, left or right of the box 0 before and
  after; every search one box (with the ink as rule 4's test, 6 of 39 were two).
- The box is exact only as far as the OCR engine's box is. The half-point pad (`MATCH_PAD_Y`) is all that stands
  between a tight engine box and a sliver of ink; the probe found none on two English scans from one engine.
- A right-to-left word is over-covered by a partial match — the whole word goes. That is the safe direction.
- `/api/pagemap` gains an optional field. A page of an older Nib ignores it and draws the reach, as it did.

## Declared gaps

- **Print still gets the line's reach.** Tightening it needs the font's own glyph boxes, or a measured constant, and a
  vertical-reach column in `build/accuracy.sh` that does not exist. /pending 851 part 1, remainder.
- **pdf.js draws a fitted word's selection and Find highlight about four points high.** That is ADR-092's stamp under
  pdf.js's text layer, found here because rule 4 needed it; nothing in this change moves it.
- **Real-scan numbers exist for English only**, two scans, one engine. The right-to-left rule rests on how the stamp
  is written (`TestARightToLeftWordIsStampedInReadingOrderFromTheLeft`), not on a real Hebrew or Arabic scan.
- **Writing a right-to-left word so that its letters ARE where the scan has them** (visual order with `/ActualText`,
  or a reversed run) would make a partial match exact and is not done.

## Guards

`TestAStampedWordSpansItsScannedBox` (the ink read back is the box handed in, every OCR face),
`TestAWordThatCannotBeFittedIsStampedAsBefore` (a short stamp is given none),
`TestOnlyAnUprightRunInAnOCRFaceHasItsInkMeasured`, `TestOnlyAHiddenWordInsideAFormIsGivenItsInk`,
`TestARightToLeftWordIsStampedInReadingOrderFromTheLeft`, and `test/jsdom/placematches.test.mjs` ("a fitted OCR word
is boxed by its own ink, and still replaces the estimate pdf.js drew above it"; "part of a right-to-left OCR word takes
the whole word").
