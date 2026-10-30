# ADR-176 — a simple font's `/Differences` are read through the glyph list, and the loss is counted

**Status:** accepted
**Date:** 2026-10-30
**Extends:** ADR-088 (`MapPage` is the one door for what a page prints), ADR-052 (`fontcode` is the one door for decoding), ADR-009.
**Supersedes:** the sentence in `textrun.go`'s header that a `/Differences` font "is NOT guessed", and the rule of
`TestASimpleFontIsDecodedOnlyThroughTheTableItNames` that it decodes nothing. **Keeps** ADR-139 (a renamed font is not boxed
to Adobe's table).
**Context:** `/pending 918`, `deepdives/2026-10-30-mappage-text-loss.md`. A simple font whose `/Encoding` is a dictionary — a
base with `/Differences` over it — says what its codes are by glyph name. `simpleDecoderFor` took only a named encoding, so such
a font gave no text and its runs were dropped by `mapText` as blank, with no error and `NoText` false. Measured on the 116 PDFs
of the 2025 IRS sources: 1,134,622 of 10,496,521 glyphs (10.8%) undecoded, in 20 files, one class; `i1040gi.pdf` lost 11,475
of 453,985 letters and digits, and the statutes and regulations 88-100%. The reason given — "no table for the names" — was
untrue: `internal/uacheck` already carried Adobe's glyph list.

**Decision.**

1. **One list, one rule, in `fontcode`** (`glyphlist.go`, `agl/glyphlist.txt` moved from `uacheck` unchanged). `GlyphList` is
   Adobe's AGL 2.0 as published; `GlyphText` is Adobe's rule for a name — drop from the first ".", split at "_", each part
   from the list, `uniXXXX…` or `uXXXX…` with upper-case hex and no surrogate half. A name it cannot read is unreadable,
   never a guess. `uacheck` builds its veraPDF list from `GlyphList` plus the one entry veraPDF adds (`.notdef`, U+0000),
   which a text reader must not have.
2. **A renamed code is the glyph it names.** `encodingDictionaryOf` (`pdfops/fontdiffs.go`) reads `/Differences` into
   `runFont.diffs`; a name the list cannot read is recorded as "", so the code stays undecoded and **never falls to the base
   encoding** (the base's character is not what is drawn — `TestADifferencesFontIsReadThroughTheGlyphList` caught the
   first cut doing exactly that). The Latin presentation ligatures U+FB00-06 are read as their letters.
3. **The base is not guessed for a symbolic font.** `/BaseEncoding` is read where it names a table nib reads. Absent, the base
   is StandardEncoding's printable range only for a font whose descriptor says nonsymbolic (`/Flags` bit 6 set, bit 3 clear);
   otherwise only the `/Differences` codes decode. Declared gap: a symbolic font's own built-in encoding is not read.
4. **`diffs` is not `simple`.** `runFont.simple` means "a named encoding or a standard font's own" and gates `coreTop`
   (ADR-139). Setting it for a renamed font tightened an unembedded core font's box to Adobe's table
   (`fontinkcore_test.go`, "glyphs renamed by /Differences" went red); a renamed font keeps its full reach.
5. **The loss is counted.** `PageMap.Unread` is the number of glyphs on the page with no text. Zero means every glyph has
   its character; above zero, "no run here" is not "nothing printed here". A caller that needs all of a page's text must read it.
6. `/Encoding 7 0 R` where 7 is a name is read as that name (`simpleDecoderFor` had taken only a direct one).

**Measured (scratch copy, then the built tree).** Corpus undecoded glyphs 1,134,622 → 28; `i1040gi` missing letters and
digits 11,475 → 27 (all U+42B2, which poppler emits for an unmapped glyph); 0 missing and 0 extra on seven other files,
`plaw-119-21.pdf` (331 pages) among them. The symbolic restriction changed nothing on this corpus.

**Declared.** Text order, spacing and word boundaries were not compared with poppler — only letters and digits as a multiset.
2024 and 2026 sources not measured. A name the list carries but the font draws differently is read as the list says (poppler
applies the same rule, so the comparison cannot see it). Reflow now attempts paragraphs in these fonts where it refused them
by cause `undecoded`; its gates, the accuracy harness (before/after below) and the tagging readers were run — see the pending
entry. A symbolic font's built-in encoding stays unread.
