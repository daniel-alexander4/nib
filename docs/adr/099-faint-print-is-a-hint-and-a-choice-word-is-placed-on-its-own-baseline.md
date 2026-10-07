# ADR-099 — faint print in a blank is a hint, not a label; and a choice word is placed on its own baseline

**Status:** accepted (2026-10-07). Extends ADR-088 (the map gains one fact), ADR-089 and ADR-096 (what stands in a
blank's way), and ADR-095 (its row rule, carried to the detectors it did not touch).

## Context

Two things ADR-095 and ADR-096 left open (/pending 851, residue sweep).

**Six date fields on the IRS 1040.** ADR-096 recorded "six f1040 date fields missed (their grounds carry placeholder
print)" beside "six grounds proposed with no real counterpart". Read from the harness's rows and a probe over the
same map, they are **not the same six**:

- The misses are two white grounds at the top of page 1 (64.8 x 12pt, "tax year beginning … ending …"), each holding
  **three** real fields — month, day, year: 417.6–432, 439.2–453.6, 460.8–482.4. Each ground holds the print
  `MM     `, `DD     `, `YYYY` at 7pt, and no divider: nothing is drawn between the three fields. `blanksIn` saw print
  filling the ground's band and proposed nothing, as it does for any cell full of print.
- The six unmatched proposals are amount-column grounds (504–576) on lines that take no entry. Not touched here; see
  the gaps.

What separates that print from a label is in the content stream: the form sets it `0.753 0.753 0.753 rg`, and every
label `0 0 0 rg`. Measured over the accuracy corpus (19 documents, first six pages each, about 11,000 visible runs with a
colour the walk carried): the six date hints are the **only** runs between 0.7 and white. Two runs are white (the
W-9's "Part I" / "Part II", headings on black bars); the lightest label is 0.651 (a greyed revision date, 20 runs on
one form); 205 runs on one form have a colour the walk could not carry (set through an `ExtGState`).

**The choice detectors.** ADR-095 found that a `buildTextRows` row is not one line — an item joins a row when its
baseline is within 6 (or 0.6 of its height) of the row's first item's, and `row.y` is that first item's — and fixed
the search estimate. `findYesNo`, `extractChoices` (so `findCircleOne` and `findPipeChoices`), `findSlashTemplates`
and `findRunChoices` still drew every choice box at `row.y`, `findRunChoices` tested the table's cells at `row.y`,
and `findCircleOne` measured "is the next row adjacent" between two rows' first baselines. By construction a choice
set lower than the first thing on its row is circled up to 6 too high, with the box's foot above the word's baseline.
**On the corpus it moves nothing**: the five detectors run over every mapped page's visible text (map runs standing
in for pdf.js items, at canvas scales 1 and 2.6) find 74 choice groups, and old and new place every box alike —
though 578 of 2,623 rows hold a second baseline and 5,828 of 35,578 words are more than a point off `row.y`. The
defect is real in the code and unobserved on these documents.

## Decision

1. **The map says which print is faint.** `MapText.Faint`: text that is filled (render modes 0 and 2 and their
   clipping twins), not hidden, in a device colour at least `faintFrom` = 0.7 of the way from black to white and not
   white. Lightness is the grey level, `0.299 R + 0.587 G + 0.114 B`, or the same of a CMYK colour's RGB. A colour
   the walk could not carry is not faint. White is not: white print is set to be read on a dark bar.
2. **Faint print is a hint. It stands in no blank's way** — `proposeFields` leaves it out of the print that blocks a
   cell, a line or a ground — **and it says how a blank is divided** (`byHints`, called from `blanksIn`, which is the
   one door all three sites already go through): one hint, however many words, leaves the blank whole; two or more
   set further apart than their own size are one field each, as wide as the hint's letters and 1.5pt more, the outer
   two reaching the blank's edges. The hint's letters are read from its glyph boundaries, so the spaces a run is set
   with ("MM     ") are not part of it.
3. **Print in ink is still a label**, wherever it is: nothing here weakens ADR-089/096's "a cell holding print is not
   a blank". Content is not consulted — "MM" in black is a label.
4. **A choice word is placed on the baseline of the item that drew it.** `buildTextRows` puts that on each word
   (`y`); a character's is `rowBaselines(row)[i]` (ADR-095's door, unchanged). No detector draws at, or tests a cell
   against, `row.y`; `findCircleOne` measures the distance to the next row from the marker's own baseline to the
   nearest item in that row. `row.y` remains what tells rows apart.
5. **The harness counts a field on a hint beside the labels, not among them** (`onHint`): the form's own field covers
   its hint, as it covers the "-" in a phone number.

## Consequences

Measured, same corpus and harness, before → after (`build/accuracy.sh`, 19 documents, 43 pages):

- Real fields found: 672 → **678 of 682** (98.5% → 99.4%); with half their area in common 670 → **676** (98.2% →
  99.1%). The 1040's first page: 122 → **128 of 128**, every one well placed.
- Proposals on answer-key pages 765 → 771, the six new ones each a real field (right 87.8% → 87.9%).
- On a label: 23 → 23 of the proposed text fields (1.5%). The six new fields each stand on their hint and are counted
  as `onHint` 6; by the harness's old rule, which took a hint for a label, the figure would read 29 (1.8%).
- Through a rule 10 → 10. Zoom disagreements 0 of 18. Search-redaction: every figure identical (187 words).
- No other page changed by a proposal. **One document read wrong in the full after-run and was re-run**: the three
  pages of the outlines form (no text; the picture's reading alone) came back with 0 proposals against 60 / 87 / 81,
  while a Go suite was running beside the harness; alone, on the same tree, they read 60 / 87 / 81. The figures above
  use the re-run rows. The harness reporting 0 for a page it pressed Detect on too early is a fault of the harness.
- Read from the map alone (a probe over the same pages, no picture): 668 → 674 of 682.

Red-proof: 37 mutations, one condition each (26 in `web/detect.js`, 11 in the map), all red for an assertion. Two
first survived and were closed: "all print is a hint" (now: two labels in a cell's top do not divide it), and a
`!hidden` on the hints that the map makes impossible (removed).

## Declared gaps

- **A hint whose glyph boundaries are not known** (a turned page, a run that does not read left to right) is taken
  whole, trailing spaces and all; hints that then touch read as one, and the blank is one field.
- **The threshold rests on one form.** 0.7 sits between the lightest label measured (0.651) and the one hint colour
  measured (0.753). A form that prints its hints darker than 0.7 is read as before; one that prints LABELS lighter
  than 0.7 would have them read as hints, and a field proposed over them.
- **A colour the walk cannot carry** — set through an `ExtGState`, a pattern, an ICC or separation space — is never
  faint. Transparency is not read at all: black at 20% opacity is ink.
- **The picture's reading (`refineFields`) still treats a hint as print.** It only adds where the map proposed
  nothing, and where a hint is the map now proposes.
- **The six 1040 grounds with no real field are still proposed.** Each is an amount-column ground on a line that
  takes no entry; what the page draws differently there is the line-number cell beside it, which is shaded and empty
  (482.4–504) where a line that takes an entry prints its number ("1z", "4b"). A rule from that would be a rule about
  one form, with nothing else in the corpus to measure it against. Not built.
- **A row holding two baselines is still read as one interleaved string** by the choice detectors, as by the search
  estimate (ADR-095's open item): a list on the lower of two merged lines can have the upper line's words between its
  options. Placement is right for whatever is found.
- `isWhitePaint` knows white only as `1 g`, `1 1 1 rg`, `0 0 0 0 k`: a ground painted white through `cs`/`sc` is not
  a ground. Found reading for this change; not measured on the corpus.
