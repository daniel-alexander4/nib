# ADR-052 — shown bytes are read through one door, and the checker reads CMaps as veraPDF does

**Status:** accepted

## Context

Two packages read what a text-showing operator shows. `internal/pdfops` reads runs of text to place,
measure and tag them; `internal/uacheck` judges PDF/UA-1 7.21.7, whose object in veraPDF is the `Glyph` —
one per character code a `Tj`, `TJ` or `'` draws. Until P07.S02 the checker did not read shown bytes at all:
7.21.7 t1 asked whether a FONT had a `/ToUnicode`, and passed a font whose CMap existed but did not map a
code the page drew (`/pending 657`).

Reading glyphs means decoding string operands, cutting them into codes by the font's codespace, and
looking each code up in a `/ToUnicode` CMap. `pdfops` already did the first and last for its own purposes,
and ADR-009 says a rule held at two sites is written once.

But the two readers do not want the same CMap reading. veraPDF-parser 1.30.2 — which the checker is
measured against — reads a CMap in ways a text extractor must not copy:

- a `beginbfchar` list maps exactly the COUNT it announces, and a list with no count maps nothing;
- a `bfrange`'s end is the begin code's leading bytes with the end code's last byte, so `<0000> <FFFF>` is
  256 codes;
- an incrementing range whose destination starts with a zero byte yields ONE character, its last byte;
- an entry of the wrong kind makes the parser throw, and the whole CMap is replaced by an empty one;
- a code is the integer value of its bytes, so a two-byte `<0041>` source maps a one-byte code 0x41 — and
  a Java `int`, so a four-byte code at or above 0x80000000 is negative and no range can hold it;
- a `/ToUnicode` stream whose dictionary `/CMapName` begins `Identity-` is identity, whatever it holds, and a
  `/UseCMap` in that dictionary overwrites the stream's own entries;
- a `usecmap` in a program merges the used CMap's codespace where it stands, so after `/Identity-H usecmap`
  the program's own one-byte ranges overlap it and are dropped.

Each was read from source and each changes a verdict (`fontcode_test.go`, and `glyphs_test.go`'s veraPDF
measurements).

## Decision

**`internal/fontcode` is the one door for reading shown bytes**: string and name decoding, the codespace
code reader (veraPDF's `CMap.getCodeFromStream`), and the `/ToUnicode` CMap. Both packages import it; `TestTheReadersRouteThroughThisDoor` fails if any package under `internal/`
declares one of the decoders it replaced, or holds a string literal naming a CMap list, outside the exemption
below.

**One reader stays outside, by name**: `pdfops.pdfcpuToUnicode` and `recountBFChar` (the stamp writer) read and
repair a ToUnicode map in the exact shape **pdfcpu** writes, to refuse what pdfcpu would refuse. That is a third
program's reading, not a copy of either of these, and the guard lists it with that reason — an exemption that
stops matching fails the guard, so it cannot outlive the reader it was granted for.

**It holds TWO readings of a `/ToUnicode` CMap, side by side, and says so**: `ToUnicode` is veraPDF's and the
checker uses it; `TextMap` is the specification's lenient reading and `pdfops` uses it. They share the
tokenizer and every decoder. This is ADR-009's named exemption rather than a violation of it: the checker's
job is to agree with veraPDF, and a text extractor that adopted veraPDF's quirks would lose real text from
sloppy producers.

**Where the checker cannot reproduce veraPDF, it refuses.** A `/ToUnicode` veraPDF would throw on is
`CannotCheck`, not an empty CMap — nib's tokenizer is not veraPDF's PostScript interpreter. A glyph whose
Unicode needs data nib does not carry (Adobe's UCS2 CMaps, a predefined CMap's codespace) is `CannotCheck`
naming which.

## Consequences

- 7.21.7 is judged per glyph, and t2 (no U+0000, U+FEFF or U+FFFE) is checked: 97 of the 106 rules.
- The simple-font fallback (`PDSimpleFont.toUnicode`) needs veraPDF's encoding tables and the Adobe Glyph
  List, which `internal/uacheck` now embeds — the list vendored unmodified from Adobe, with the one entry
  veraPDF adds (`.notdef` → U+0000) written in code beside its reason.
- `pdfops` keeps its own lenient cut of an Identity string (two bytes, a trailing byte alone): veraPDF completes
  a string ending inside a code with 0xFF and judges a glyph nobody drew, which the checker must do and a text
  extractor must not. The first draft moved `pdfops` onto the veraPDF reader and read a width and text for CID
  0x41FF; the review caught it.
- The checker binds a font at `Tf` (and at `gs` with an ExtGState `/Font`), not at the show, and a tiling pattern or
  Type 3 procedure inherits the font it was entered with — each a false pass the review measured before it was fixed.
