# ADR-089 — a field is read from the lines the page draws, and the picture adds only what they do not show

**Status:** accepted (2026-10-06). Step four of ADR-088's five. Extends ADR-088; supersedes nothing.

## Context

ADR-088 gave detection a map of the page and used it to *correct* what a picture of the page proposed
(`refineFields`). That fixed where proposals were wrong — on a label, through a rule, a different answer at a different
zoom — and left alone what the picture never proposed. On the nine answer-key pages it still found 62.9% of 682 real
fields.

The two worst pages were ordinary table forms: 18 of 70 and 16 of 78 found. Traced (a probe that dumps the raw and the
corrected proposals beside the map): 47 and 54 of their text fields were **never proposed at all**. A cell with a label
printed in its top is not blank by ink, so the picture's "blank cell" pass skips it — and that is what nearly every
cell of a form is.

Two things in the instrument were wrong and are corrected with this step, because the step is judged by it:

- **"Found" was generous.** A proposal found a real field when either held the other's centre, so a 15pt sliver inside
  a 200pt field counted. Step 2's apparent regression on those two pages (23 → 18, 24 → 16) was eight and five such
  slivers being removed; the count of fields with half their area in common did not move (18 → 18, 16 → 16). The
  harness now reports `foundWell` (IoU ≥ 0.5) beside `found`.
- **"On a label" counted the form's own separators.** The `-` of a phone number and the underscores of a typed blank
  are print, and the real fields of the corpus's forms cover them. `labelsOf` now takes a run's letters and digits,
  split at a run of four underscores. Step 2's 1.9% was measured the old way; the figure below is the new one.

## Decision

1. **`proposeFields(map)` reads a page's fields from what it draws** (`web/detect.js`), on any page the map has text for:
   - a **cell** is the room between two ruled lines and the uprights that join them; its field is below a label
     printed in its top and beside any print that shares its band, and a cell with two blanks gets two;
   - a line that is no row's edge is **a line to write on**, one line's height above it;
   - a **run of four or more underscores** is a blank; a **small empty square**, or a box drawn as a character, is a
     checkbox, and a cell holding tick boxes gets no text field.
2. **Lines are read the way forms draw them.** Pieces that continue one another are one line (a row's edge is commonly
   one piece per cell, an upright one piece per row); a thin filled bar is a rule; a bar heavier than 1.6pt divides
   sections and is not written on; each stretch of a line is closed by the nearest line below it, so a cell two rows
   deep is found; a table's top edge is not a line to write on.
3. **Print that is only separators is not a label.** It stands inside a blank.
4. **The picture adds only where the map proposed nothing** (`mergeProposals`). The picture's proposals are still
   corrected by `refineFields` first. Where both see a field, the map's edges are the page's own.
5. **No label, no map reading.** A page whose text is outlines (`noText`) or a scan keeps the picture alone: without
   text a labelled cell cannot be told from a blank one.
6. The message says how many fields were read from the page's own lines.

## Consequences

Measured, same corpus and harness, before → after (`build/accuracy.sh`, 19 documents, 43 pages):

- Real fields found: **62.9% → 91.2%** of 682; with half their area in common: **56.0% → 84.5%**; mean IoU 0.70 → 0.80.
- Proposals that are a real field (answer-key pages): 70.6% → 77.1% (608 → 807 proposed).
- The two table forms: 18 of 70 → **70 of 70**, 16 of 78 → **78 of 78**. The checkbox grids: 166 → 201 of 201, and
  16 → 32 of 40.
- On a label 1.5% of 1,644 proposed text fields; through a rule 0.1% (one, on a W-2).
- Zoom changes nothing on 18 of 18.

**Declared gaps.**

- **An IRS 1040 is unchanged (98 of 128, 67 of 71).** Its amount boxes are bounded by shading and by lines of unequal
  extent with no uprights; the map reading proposes 6 fields there and the picture does the rest. The remaining misses
  on that page are the picture's.
- **A table of text draws a few fields.** A cell holding one short line of print has room beside it, and that room is
  indistinguishable from "From month/year: ____". A W-9's instruction page is given 13 (it was given 109 before
  ADR-088, and 1 after step 2). A rule that refused cells holding two lines of print was measured and dropped: it cost
  ten real fields on one form and saved two here.
- **A box glyph is recognised only by its Unicode character.** A checkbox set in a symbol font with no `/ToUnicode`
  reads as a letter; `fda-135045` is unchanged for that reason.
- The corpus is one machine's. The thresholds (a 45pt row, a 24pt second blank, a 20pt square) are fitted to it.

Guards: `test/jsdom/proposefields.test.mjs` — fifteen cases stated as numbers; twenty conditions of the rule were each
weakened in turn and each turned a named test red. Whether real forms come out better is `build/accuracy.sh`'s to say,
and not a tier's.
