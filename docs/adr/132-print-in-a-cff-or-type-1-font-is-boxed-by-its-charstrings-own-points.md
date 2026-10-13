# ADR-132 — print in a CFF or Type 1 font is boxed by its charstrings' own points

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-098 (print is boxed by the ink its font's own glyphs reach) from embedded TrueType to CFF and
Type 1 programs. ADR-088, ADR-093 (`Rect` stays the reach; `Ink` sits beside it). ADR-009.
**Context:** /pending 851 parts 1 and 2. ADR-098 set `MapText.Ink` only from a `/FontFile2` program, and CFF and
Type 1 were its declared gap — 73% of the corpus's print runs, both IRS forms — so a search-redaction box in those
fonts still reached a full size above the baseline and took ink from the line above.
**Applies:** `internal/pdfops` (`fontinkcs.go`, `fontink.go`, `printInk`), the page map's reader only.

## Decision

**1. `fontinkcs.go` runs Type 2 and Type 1 charstrings and keeps the lowest and highest y any point of the outline
names, control points included.** That is never less than the ink. A move nothing draws from, and a Type 1 flex's
reference point, are not ink.

**2. It is a second reading beside `internal/uacheck`'s, not a shared parser.** That one answers "what will veraPDF
say" and copies veraPDF's misreadings on purpose; this one answers "where does a renderer paint", and is written
from Adobe's Technical Notes #5176 and #5177 and the Type 1 book (so ADR-114 asks no header of it). The split is
the one `fontcode`'s two CMap readings already declare. Neither package imports the other.

**3. Which answer a font gives is ADR-098's rule, unchanged.** A simple font (Type1 over `/FontFile` or
`/FontFile3`) answers for its whole program, and only to tighten. A Type0 font under Identity-H over a
CIDFontType0 answers for the run's own glyphs, by the charset (CID = glyph for a name-keyed program), and may grow
the box. A program under a key its subtype does not draw from is not read.

**4. A font is vouched for whole or not at all.** Refused: an operator with the wrong argument count, arithmetic
or storage operators, a four-argument `endchar`, a `seac` part outside the letters and accents or itself accented,
an unknown OtherSubr, a second `/Subrs`, a font matrix that is not a thousand to the em upright, or any bound —
subroutine depth 10, stack 48 (Type 1: 24), 100,000 steps a glyph, 8,000,000 a font, 65,535 glyphs, 256 font
dictionaries. Refusal means no ink: the line's reach stands, a box too tall and never too short. After one
unreadable glyph a CID font gives no ink to later runs.

## Measured

`build/accuracy.sh`, 46 pages, 6,490 words, before → after: a box reaching the line above 399 → 132, below
55 → 53, **ink outside a box 0 → 0**, no page worse; detection and redaction columns identical. Against fontTools'
control bounds: all 743 glyphs of the corpus's 20 embedded programs and 127,168 glyphs of 180 installed fonts
(47 Type 1, 133 OpenType-CFF) agree. At most 0.37 ms per corpus program; 23 ms for a 734 KB math font.

## Consequences and declared gaps

- The corpus holds no EMBEDDED Type 1 program (its 61 "Type 1" fonts are unembedded), so that half rests on
  fixtures and installed fonts, with no measurement against a rendered page.
- A simple font's codes are not mapped to glyphs; it tightens by the whole program.
- Not built, refused instead: `endchar`-as-accent in CFF, arithmetic operators, multiple masters.
- An unembedded font keeps the full reach — that is the remaining 132.
- No font file is committed; fixtures are built in code.
