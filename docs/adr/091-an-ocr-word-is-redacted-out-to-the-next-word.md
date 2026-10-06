# ADR-091 — an OCR word is redacted out to the next word, because the stamp is narrower than the ink

**Status:** accepted (2026-10-06). Supersedes ADR-090 §4 for hidden text; the rest of ADR-090 stands.

## Context

ADR-090 placed search-redaction boxes from glyph boundaries and left hidden text — an OCR layer — on the old estimate,
reasoning that a stamped glyph says where the invisible word was put, not where the scanned ink is. Step six of
ADR-088's plan was to make the two agree by stretching each stamped word to its scanned width. Measuring before
building it found that the disagreement was already a defect, and not in the stamp's reader of the future but in the
redaction of today.

`pdfops.StampTextLayer` sets each OCR word at its box's bottom-left, at a font size equal to the box's HEIGHT in whole
points, in Roboto. The engine's box is the word's ink, so the size is an x-height or a cap height, not an em, and the
word comes out narrow. Measured by running OCR in the real app on two scanned documents (3,855 words) and comparing
each word the engine reported with the map of the layer that was written:

- stamped width ÷ scanned width: **median 0.72**, tenth percentile 0.56; within ±10% for 4-15% of words by page;
- the stamp's right edge is a median **3-4.5pt short** of the ink, 9-12pt at the ninetieth percentile;
- the stamp's box ends a median 0.75pt above the bottom of the ink.

The estimate padded a match by 0.8 of a line height, and the line height was the same too-small size. **Observed, in
the app, on an OCR'd scan: 30 of 40 search-redactions left the end of the scanned word outside the box — by up to
13.8pt, 13.6pt of a 53pt word — and 7 left ink below it.** Applying those redactions flattens the page under the box
and leaves the rest: the word's last letters stay in the document. This predates ADR-088; it is in every release that
has had *Redact text…* and OCR.

## Decision

1. **A hidden run takes part in `matchesInMap`**, as a layer of its own: print and an OCR layer are searched
   separately and never joined into one line.
2. **A hidden run's glyph boundaries are stretched from where it starts** — the one thing a stamped word has right —
   toward the next hidden run on its line: to half a point short of it, never more than **2.5×** its own width (the
   ninety-ninth percentile of what was needed is 2.14), and never less than 1. The last word on a line has no next word
   and takes the ceiling.
3. **Its box is a quarter of the font size taller, above and below.**
4. **A layer that already spans its words is unchanged**: a run that reaches its neighbour has no room to stretch into.
   That is how another tool's OCR layer, stretched when it was written, reads exactly as before.
5. `placeMatches` is unchanged. A stretched box holds its estimate's centre and replaces it.

**Why the reading and not the stamp.** Stretching the stamp fixes documents OCR'd from now on. It cannot reach a
document OCR'd by any earlier Nib, and those are the documents people already have. The reading reaches both.

## Consequences

- In the app, same scan, same 40 words: ink past the box **30 → 0** on the right, **7 → 0** below. The box runs a
  median 4.3pt past the ink — into the space before the next word.
- Against every OCR'd word of three or more letters on both scans, through the shipped function: ink past the box in
  **15 of 2,995 (0.5%)**. Each is a word whose reported box overlaps the start of the next — the engine boxing a
  fill-in rule together with its label, or 4-6pt print. Raising the ceiling to 4 moves 15 to 11; the next word, not the
  ceiling, is what binds.
- The last word on a line is over-covered by up to 2.5× its stamp (ninetieth percentile 12.9pt past the ink): paper.

**Declared gaps.**

- **The stamp is still narrow.** Find's highlight, a text selection and a copy in any viewer still follow the narrow
  glyphs, and **another program's search-redaction of a Nib-OCR'd document has the defect this fixes here.** Stretching
  the stamp (ADR-088's step six) is still owed and is now a fix, not a refinement.
- **A hand-drawn redaction box is the user's and is not checked against anything.**
- A run without glyph boundaries (turned text) still takes the estimate, hidden or not.
- Measured on two scans, one language (English), one engine. The 2.5 is fitted to them.
- `build/accuracy.sh` does not score redaction on scans — it has no words to search before OCR. The figures above came
  from a probe (OCR in the app, then search); making it a column of the harness is owed.

Guards: `test/jsdom/placematches.test.mjs` — the four hidden-layer cases; thirteen conditions of the stretch each
weakened in turn, each turning a named test red (no stretch, no ceiling, may shrink, past the next word, layers
joined, print stretched).
