# ADR-096 — a white rectangle is the ground under a blank, and the map keeps it as that

**Status:** accepted (2026-10-06). Extends ADR-088 and ADR-089; supersedes ADR-088 §3's "a piece painted white in a
device colour is not in the map" for white RECTANGLES only, and closes ADR-089's first declared gap.

## Context

ADR-089 left an IRS 1040 unchanged — 98 of 128 and 67 of 71 real fields found, the map reading proposing 6 — and
recorded the cause as "amount boxes bounded by shading and by lines of unequal extent with no uprights". Traced (a probe
that dumps every painted piece with its paint, beside the form's own fields), that is not what the page draws:

- The page lays ONE tint over itself first (`0.863 1 0.984 rg`, two rectangles 492pt wide, 30pt and 708pt deep).
- Then **137 rectangles painted white**, one under each blank: 72 x 12 under an amount, 8 x 8 under a tick box,
  24 deep where the label is printed in the blank's top. They agree with the form's 128 real fields to the point.
- `MapPage` dropped every one of them (`s.white`: "on a white page it is not there"), kept the tint as a `Filled` box,
  and `proposeFields` refuses anything whose centre is inside a `Filled` box. So the map threw away the form's own
  statement of where its blanks are and then refused the page for being shaded.

The same trace found a second, wider defect in the same field. `Filled` is documented as "painted with something
other than white", and was set from "the path was filled" — so a box **outlined in ink and filled white** was a shaded
panel. Two forms in the corpus draw every cell that way (51, 24, 24 and 19 boxes on four pages); the map proposed
nothing inside any of them and the picture did that work.

With `Filled` corrected those cells became lines, and exposed a third: boxes stacked in a column 6pt apart. A row is
closed by "the nearest line below it", lines within 7pt being skipped — so the bottom edge of one box skipped the top
edge of the next, paired with ITS bottom edge, and proposed a second field over the cell (48 on one page).

## Decision

1. **A rectangle painted white and nothing else is in the map as its own kind, `"white"`.** Not `"box"`: a reader of
   lines and boxes must not see it, and none does — every existing reader names the kind it wants. A white RULE (a
   piece thin enough to be a line) erases part of another line and is still dropped, as is a rectangle only outlined
   in white. A ground that a later piece paints a colour over, whole, is dropped: it is not white on the page.
2. **`Filled` means a colour.** A box whose inside is painted white is an empty box, whatever its outline.
3. **`proposeFields` reads a white ground as a blank**, after the lines and only where they proposed nothing: a ground
   up to a row deep (45pt) is a text field below a label printed in its top and beside print that shares its band
   (`blanksIn`, the cell's own rule); a small square one is a tick box, unless print stands in it; the smallest are
   read first, so a ground holding a tick box or another ground is taken by what it holds. **A ground is the one
   thing proposed inside a shaded panel** — it is painted over the shade.
4. **A line just under another closes what it covers, with no room.** The lower line opens its own rows.
5. **Nothing was built for a checkbox set in a symbol font**, ADR-089's third declared gap, because the corpus has
   none. `fda-135045`, which that gap names, is four pages of prose: its only symbol font (OpenSymbol) has a
   `/ToUnicode`, every run on every page decodes, and its 58 symbol glyphs are one code that reads U+2022 at 0.355 em —
   list bullets, confirmed on a rendering. The other symbol-font glyphs in the corpus are bullets too (Symbol U+F0B7;
   Wingdings U+F0D8 and U+F0A7). A rule keyed on "one glyph in a symbol font" would have nothing to be measured
   against and three bullet shapes to fire on, so `MapText` does not carry the font name.

## Consequences

Measured, same corpus and harness, before → after (`build/accuracy.sh`, 19 documents, 43 pages):

- Real fields found: **91.2% → 98.5%** of 682; with half their area in common: **84.5% → 98.2%**; mean IoU 0.80 → 0.85.
- Proposals that are a real field (answer-key pages): 77.1% → 87.8%, from FEWER proposals (807 → 765).
- The 1040: 98 → **122 of 128** found, 91 → 122 well placed; its second page 67 → **71 of 71**, 66 → 70.
- The two forms whose cells are outlined boxes filled white: 201 of 201 found before and after, **175 → 201** well
  placed, from 256 → 210 proposals; 32 → **40 of 40**, 28 → 40 well placed (two pages); 25 → **31 of 31**, 22 → 31.
- Read from the map alone, with no picture (a probe over the same pages): 360 → **668 of 682**, every one well placed.
- No page of the other fourteen documents changed by a single proposal.
- On a label 1.6% → 1.5% of proposed text fields. **Through a rule 0.1% → 0.6%** (1 → 10): the nine new ones are
  all on the 1040, and each has an IoU of 0.91 or more with a real field — a comb's dividers cross one field. Cutting a
  ground at its uprights was built and measured: it lost seven well-placed fields and gained none, and is not done.
- Zoom changes nothing on 18 of 18. Search-redaction is untouched (identical figures).

**Declared gaps.**

- **A white ground on a white page is still read as a blank.** That is what the 1040's grounds outside its tint are,
  and what a designer-made form's are; a document that paints white rectangles for another reason — to erase, to back
  a table of text — is offered fields there, less whatever print stands in them. On this corpus that is four tick
  boxes on one page of a membership form (white 11pt squares in a column), read from the map alone: the harness
  timed out opening that document before and after this change, so the browser did not measure it.
- **Six of the 1040's grounds have no field** (the amount column beside its shaded line-number boxes, on rows whose
  amount goes in the inner column). The form drew a blank there; nothing in the page says it is not one.
- **A ground with print standing across it is not a blank.** The 1040's two dates are three fields each on one ground
  that prints a placeholder in every one; `blanksIn` finds no room beside that print, and those six fields are all of
  the page's remaining misses.
- **Paint order is paths only.** An image or a shading drawn over a ground is not modelled, and a ground hidden by one
  is still read.
- The corpus is one machine's.

Guards: `internal/pdfops/pagemap_test.go` (`TestAWhiteGroundIsInTheMapAsAGround`), `test/jsdom/proposefields.test.mjs`
(seven cases). Thirty-three conditions were each weakened in turn and each turned a named assertion red; two that would
not — a second "cell of tick boxes" refusal and a separate list of what stands in a ground's way — were inert and are
removed.
