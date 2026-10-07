# ADR-097 — a right-to-left OCR word is set where its letters are, and a text-layer span is its line's em box

**Status:** accepted (2026-10-07). Extends ADR-092 (the stamp is fitted to its scanned box) and ADR-093 (part of a
right-to-left OCR word takes the whole word). Supersedes ADR-093 rule 5 for a Hebrew word written as rule 1 says, and
closes its fourth declared gap; corrects its second, which named a cause that is not the cause. /pending 851 parts
3 (precision) and D.

## Context

**The order.** `StampTextLayer` handed pdfcpu each OCR word in reading order, and pdfcpu sets a word glyph after glyph
from the left of its box. For Hebrew and Arabic that put the word's first letter at the left end of the stamp and the
right end of the scan. ADR-093 closed the leak this made — part of such a word was boxed at the mirrored end — by
taking the whole word, and recorded the precision as open.

Measuring what reads the stamp found a second defect nobody had recorded. A PDF sets right-to-left text in the order
it is SEEN, and every reader turns it round to read it. Handed a word already in reading order, they turned it
backwards: `pdftotext` gave "םולש" for "שלום", and so did pdf.js's `getTextContent` (the app's own build, in a real
browser) — which is what Find, copy and the estimated half of a search-redaction read. Only Nib's own map, which reads
the bytes as they are, had the word the right way. The comment that said otherwise (`ocrFontFor`: "only reverses them
on display") was a description, not a measurement.

**The span.** ADR-093 recorded that pdf.js draws a fitted word's text-layer span "about four points above its ink",
put it down to the fit scaling across and up differently, and listed moving it as open. Measured: the cause is not
the fit.

## Decision

1. **A right-to-left word is set in the reverse of reading order, inside `/ReversedChars BMC … EMC`.** That is the
   order print sets it in, so glyph i from the right is letter i, and `ReversedChars` is what ISO 32000-1 §14.8.2.3.3
   provides for exactly this: a show string whose characters are in the reverse of reading order. The bracket goes
   inside the word's own form, around the show string, and not on the page around the `Do`: the tagger replaces the
   page's marker, and "within the sequence" is unambiguous there.
2. **Only a word that runs one way** (`setOrder`): at least one right-to-left letter, and no Latin letter, no digit
   of either kind and no bracket. Those keep their own direction inside a right-to-left word or may be mirrored, and
   a reader no longer simply turns the word round (measured in poppler: "ב-2020" and "م2" set in reverse came back
   wrong). Such a word is set as before — reading order, unmarked — and is read as before.
3. **No `/ActualText`.** Every reader measured already does the turning; replacement text would be a second answer to
   a question they have answered, and Nib's own walker treats a sequence carrying it as text it must not rewrite.
4. **The walker knows the tag and the map turns the word round** (`textRun.reversed`, `MapText.Reversed`): `Text` and
   `Chars` are in reading order and `Cuts` run from the right — `Cuts[i]..Cuts[i+1]` is still `Chars[i]`. The tag is
   read for print too. Nothing else that reads a run is changed: reflow, the tagger and the checker see the order the
   page sets, as they do for every print right-to-left document.
5. **A partial match is boxed by its glyphs only in a Hebrew word written this way and fitted** (`lettersWhereScanned`,
   web/detect.js). Everything else right-to-left and hidden is still taken whole: a layer stamped before this, a short
   stamp, another tool's layer — and **an Arabic-script word, written this way or not**. Its letters are stamped in
   their separate forms and printed joined; measured on the face it is stamped in (30 words, the isolated forms against
   the face's own joined forms, both scaled to one width), the letter boundaries are a median 0.33 of a letter apart,
   0.78 at the ninetieth percentile, and off by more than half a letter somewhere in 23 words of 30.
6. **The estimate lays a right-to-left item out from its right** (`buildTextRows`, from pdf.js's `dir`). It laid every
   item out from the left, so part of a Hebrew or Arabic word in PRINT — which the map does not match — was boxed at
   the mirrored end: ADR-093's leak, in the reading it did not touch. It is also what lets rule 5's exact box replace
   its estimate instead of standing beside it.
7. **Nothing is built in the stamp for pdf.js's span, and `hold` stays.** A text-layer span is the line's em box:
   pdf.js sets its top at the baseline plus the FALLBACK font's ascent (0.818 of the size for sans-serif in Chromium)
   and makes it one size tall. The size is the product of `Tf` and the matrix, which ADR-092's ink-on-the-box rule
   fixes; no split of it between the two changes what pdf.js computes, and no stamp that keeps the ink on the box can
   move the span. A print word of the same size gets the same span.

## Consequences

- A Hebrew or Arabic OCR'd scan is now found by Find, copied the right way round, and read the right way by poppler
  (measured: three Hebrew and three Arabic words, each reversed before and correct after, in both readers).
- A search for part of a Hebrew OCR word covers those letters and not the whole word.
- `/api/pagemap` gains an optional field. A layer stamped before this has no tag, so nothing about it changes.
- Measured for rule 7, on two real fitted layers (40 and 39 words, the engine's boxes) and a synthetic one: the
  span's height is 1.00 of the size and its top 0.82 of the size above the baseline on every word, fitted or print; it
  stands a median 0.71pt and 0.49pt above the ink's top on the real scans' words (tenth to ninetieth percentile
  0.58–0.99 and 0.41–0.66), and 0.9–3.3pt on synthetic words — 3.3pt is a word with no ascender, where the ink stops
  at the x-height and the em box does not. The same print word at 14pt: 3.9pt.

## Declared gaps

- **No real right-to-left scan was measured.** The evidence is synthetic stamps in the faces Nib stamps in, read by
  Nib's map, poppler and pdf.js, and the search driven in the real app. It is not evidence that a scanned Hebrew
  typeface's letters are as wide as Noto Sans Hebrew's — the same assumption a partial match in a fitted Latin word
  already makes.
- **Arabic stays whole-word.** Writing the joined forms (the face has all 117 presentation forms) would bring the
  boundaries closer; a lam-alef ligature and a scanned face's own widths would still move them, and it is not done.
- **A word the rule leaves in reading order is still backwards to other readers** — a Hebrew word with a digit in it.
- **A phrase of several right-to-left OCR words is not matched by the map**: its rows are read left to right, so the
  words are in reverse. The estimate, which now reads them the right way, places it.
- **An unpaired page** (ADR-092 rule 4) leaves a reversed word unmarked. The map then reads it as it reads print: in
  the order it is set, so the map does not match it and the estimate does.
- **`hold` is kept though the measurement that asked for it no longer does.** With `placeMatches` testing an
  estimate against the ink alone, 0 of 40 and 0 of 39 whole-word searches on the two real scans keep a second box
  (ADR-093 measured 6 of 39). What ADR-093 saw was the estimate drawn on its row's first baseline, which ADR-095
  has since fixed — not pdf.js's span. An estimate's centre is 0.35 of the size above the baseline, so a match whose
  ink is all above or all below that (apostrophes, a row of full stops) still misses the ink; whole alphanumeric
  words, which is all the probe searches, cannot show it. Unmeasured on such a match.

## Guards

`TestARightToLeftWordIsSetWhereItsLettersAre` (stamped and tagged; the map's order and cuts; poppler's reading),
`TestOnlyAWholeRightToLeftWordIsSetInReverse`, and in `test/jsdom`: `placematches.test.mjs` ("part of a Hebrew OCR
word set in reverse is boxed where its letters are", "…and only that") and `wrappedmatch.test.mjs` ("the estimate for
part of a right-to-left item is drawn at the end its letters are at").
