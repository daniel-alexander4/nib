# ADR-092 — an OCR word is stamped to its scanned box, and only a short stamp is read out to the next word

**Status:** accepted (2026-10-06). Step six of ADR-088's plan. Closes ADR-091's first declared gap ("the stamp is
still narrow") and supersedes its rules 2 and 3 for every hidden run that is not a short stamp; the rest of ADR-091
stands.

## Context

`pdfops.StampTextLayer` set each OCR word at a font size equal to its box's height. The box is the word's ink, so the
size was an x-height or a cap height and not an em, and the word came out a median 0.72 of the scanned width (ADR-091).
ADR-091 fixed Nib's own search-redaction by reading such a word out to the next one. It could not fix anything that is
not Nib: measured with poppler's word boxes on five synthetic scans, the same layer reads at a median 0.63-0.79 of the
ink's width, the ink running a median 1.75-7.2pt past the box. A search-redaction in another program, a selection, a
copy and Find's highlight all follow those glyphs.

pdfcpu's watermark cannot write the word wider. Its placement matrix is unit scale by construction
(`CalcTransformMatrix(1, 1, …)`), and its font size is an integer at every step, so a uniform scale lands on whole
points and there is no horizontal one.

## Decision

1. **Each word is fitted to its box after pdfcpu stamps it**, in the same rewrite, by multiplying a fit into the six
   numbers of the `cm` pdfcpu placed the word's form with. The fit is applied in the form's own space, ahead of that
   matrix, so where the word goes — on a turned page too — is still pdfcpu's and `stampInPlace`'s decision.
2. **Across, the word's advance spans the box.** That is the extent every reader computes a glyph run from. Fitting
   the ink's left and right edges instead leaves a script whose marks are drawn outside their advance short (measured:
   Gurmukhi, 10pt).
3. **Up and down, the glyphs' own ink is put on the box's top and bottom** — the ink of this text as this face draws
   it, read from the face's glyph outlines (`x/image/font/sfnt`; pdfcpu keeps advance widths only). That gives the word
   its size and its baseline with no fitted constant and no per-script table.
4. **Words are paired to what pdfcpu wrote by position, checked by count** — the rule the tagger already states.
   pdfcpu hands back nothing that names a word's form. A page whose counts disagree is left as stamped.
5. **A word that cannot be fitted is stamped as before**: no ink in its face, an empty box. The route logs how many.
6. **The map marks a short stamp** (`MapText.Short`): hidden text in a form drawn at one scale both ways, which is
   what Nib wrote before this and what rule 5 still writes. **ADR-091's reading — out to the next word, a quarter of
   the size taller — now applies to a short stamp only.** Every other hidden run is read as it is written, as print
   is, on a layer of its own.

**Why the reader had to change with the writer.** ADR-091's extra height is a quarter of the font size. On a fitted
layer the size is the true em, the box is already the line's, and a quarter more reaches into the lines above and
below: on synthetic scans the padded box touched a neighbouring line's ink for 50-73 of 74 words, against 0-31 with
none — and no ink of its own outside it either way.

**Why a mark and not a rule about room.** A misread can go only one way. A short stamp is drawn at unit scale and
stays uniform under anything a page operation does to it, so it is never read as fitted; a fitted word whose two
scales happen to agree is read as short and gets a larger box.

## Consequences

- Synthetic scans (words placed as the true ink boxes of other faces, 74 words × 5), read by Nib's map and by poppler
  alike: width ÷ ink 0.45-0.94 → **1.00**; ink past the box's right edge up to 26pt → **at most 0.23pt**; stamped size
  ÷ true em 0.64-0.70 → **0.93-0.97**.
- Two real scans OCR'd in the app (1,717 and 3,134 words), each word compared with the box the engine reported:
  width ÷ ink 0.59 / 0.72 / 0.97 (tenth percentile / median / ninetieth) → **1.00 / 1.00 / 1.00**; ink past the
  stamp's right edge a median 3.4-4.5pt, 8.6-11.4pt at the ninetieth percentile → **0.00**, in Nib's map, in poppler's
  word boxes and in pdf.js's text layer (0.73-0.81 → 1.00 of the word's width).
- Nib's search-redaction on the new layer, 79 words: ink outside the box **0** on every side, the box a median
  0.74-0.78pt past the word on the right (2.8-4.6pt when it was carried out to the next word).
- The same two scans as OCR'd by v1.189.1, opened in this build: every figure unchanged — 0 of 79 with ink outside
  the box. ADR-091 still reads them.
- A box that reaches another line's ink: 17 → 13 of 40 and 8 → 5 of 39. Not zero: what is left is `runBox`'s own
  reach, a full size above the baseline, for print and hidden text alike.
- Cost, 592 words on one page: 512-537ms before, 512-530ms after. Bytes +1%.
- The tagger is untouched: it replaces the marker, and the matrix after it is not its business.

**Declared gaps.**

- **A document OCR'd by an earlier Nib keeps its short stamp.** Nib reads it out to the next word (ADR-091); another
  program does not. Running OCR again is the only repair, and whether that replaces the layer or doubles it is not
  established here.
- Unshaped glyphs: a script that joins or stacks is measured as its isolated glyphs. The run still spans the box.
- A right-to-left word is written in logical order running left to right, as before: a whole word is boxed
  correctly, part of one is boxed at the mirrored end.
- A word's size is fitted per word, so sizes along a line differ by what the two faces' proportions differ by
  (measured 0.88-1.00 of the true em). A line's own baseline is not used: the client does not send it.
- A box `runBox` draws reaches a quarter of the size below the baseline. A script with deeper descenders (measured:
  Arabic, 1.7pt on one sample) is short at the bottom, hidden or print.
- The page that pdfcpu could not stamp (a content stream it cannot decode) has no layer at all; that is counted now,
  not repaired.

Guards: `internal/pdfops/ocrfit_test.go` — every OCR face, words of distinct widths handed over out of order on a
page that already carries a stamp; poppler's reading; a turned page; the unfittable word. `test/jsdom/placematches.test.mjs`
— a fitted hidden run read as written, a short one as ADR-091 has it.
