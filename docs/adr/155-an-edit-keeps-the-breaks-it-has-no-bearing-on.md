# ADR-155 — an edit keeps the breaks it has no bearing on

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-071 (text that moves takes what is anchored to it, or refuses) — by moving less text. ADR-009
(one door: the breaker's hook).
**Context:** `/pending 797`, and the gutter half of `/pending 871`. An edit to one line of a paragraph re-broke
the whole paragraph with nib's own breaker. No single measure reproduces every producer's breaks: `lineMeasures`
takes the widest line under the paragraph's median space, and in fda-83122 p4 ¶1 that gives 467.9pt against an
edge room of 451.3pt, so a word was pulled up from the next line and the rest cascaded — an edit confined to
line 1 moved a word on line 3. The flush-edge measure was measured through production's path and refused (326 →
273 of 345 justified paragraphs in place, worse for every producer).
**Applies:** `internal/pdfops` (`reflow.go`: `keptLines`, `justifiedLines`), `mdpdf` (`BreakGreedy`'s `KeepLine`).

## Decision

**1. A break the edit has no bearing on is kept.** `keptLines` hands the breaker, through `BreakGreedy`'s
`KeepLine` hook, the lines the producer set: the word that began a line still begins it wherever the line ahead
stands where it stood and is no narrower than it was, and a whole kept line's words stay together. Both only
where the line has the room it had.

**2. A line set exactly as the producer set it is not asked whether it fits.** `justifiedLines` used to ask, and
the producer's own line could fail nib's measure.

**3. The measure is not "fixed" instead.** Each candidate measure moves some producer's paragraphs; keeping the
breaks needs no measure to be right.

**4. A run does not continue a line across a gutter** (`guttersOf` / `pageGutters.parts`, grouping.go; /pending
871). A run starts its own line where it begins more than 0.6 em past the line, at an x where three or more lines
begin with text beside them, nothing within 2.5 em above or below is drawn across that x, and the line holds more
than one word. Read from the page's own lines only — reflow's and tagging's readers keep no shapes (ADR-088), and
a drawn-rule test was measured and refused (1,156 of its 1,255 cuts were one document's table rows).

## Consequences

Measured on the 36-document producers corpus through the model instrument (`breakWithout`), other words moved by
an edit, before → after: last word taken away 52 → 0 of 2,067 paragraphs; first word retyped 65 → 3, first word
of a middle line retyped 65 → 2, last word retyped 65 → 1 of 2,436. `TestHowOftenARealProducersParagraphRebreaksInPlace`
asserts those (cap 3). The gutter rule parted 185 joins, every one read; lines in paragraphs that re-break in
place 10,434 → 11,448; "paragraphs" whose adjacent lines sit in different columns 781 → 201; pages read as one
column 290 → 265.

**Declared:** a line the edit makes SHORTER is still filled by the old breaker, so deleting a word on line 1 can
still cascade. Four paragraphs still move on a retype (fda-99140 p30; irs-p15 p10, p32, p53 — the natural space is
read from a last line that is itself set wide). The census's headline figure fell from 2,592 of 2,640 to 2,371 of
2,436 because 781 mixed-column "paragraphs" used to count as in place. A single word ahead of a gutter stays
joined; a contents list that sets some page numbers past the join gap now parts them all; a table inside a page
column still interleaves its cell columns as paragraphs (irs-fw9 p4).
