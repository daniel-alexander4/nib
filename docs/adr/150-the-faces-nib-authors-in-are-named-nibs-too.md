# ADR-150 — the faces nib authors in are named nib's too

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-044 (a face nib draws with is named nib's own) to the gap it declared: the AUTHORING faces.
ADR-033 is re-measured, not changed. ADR-009 (one door).
**Context:** `/pending 560`. ADR-044 renamed the STAMP faces (`Liberation…` → `Nib…`) because pdfcpu matches a
watermark's font to one already in the document by name and by nothing else. `AuthoredTextFaces` — Roboto in four
weights plus LiberationMono, the faces `mdpdf` sets a Markdown conversion in and `StampWatermark`,
`StampPageNumbers` and the co-signing readme draw with — reached the same door unrenamed, so a document carrying
a foreign `Roboto` dropped those stamps to Base-14, logged, exactly as an office document did before ADR-044.
LiberationMono in that set also carried ADR-044's OFL §3 point: pdfcpu embeds a subset (a Modified Version) and
nib shipped it under the Reserved Font Name.
**Applies:** `internal/pdfops` (`fontrename.go`, `authoringFaces`).

## Decision

**1. The authoring faces are installed as `NibText-Regular`, `NibText-Bold`, `NibText-Italic`,
`NibText-BoldItalic` and `NibMono`.** `NibMono` is the stamp set's own renamed LiberationMono — one face, one
name, both sets.

**2. Through ADR-044's door, not a second one.** `authoringFaces` takes each face from `stampFaceBytes`
(`renamedFace` on a runtime copy; the vendored bytes are upstream's, byte for byte). The tokens are ONE table,
`faceRenames` (`Liberation` → `Nib`, `Roboto` → `NibText`), read by both `nibFaceName` and the name-table
rewrite, so the name a face is installed under and the name in its bytes cannot disagree.

**3. Only the name-bearing records move** (ADR-044 decision 2): copyright, trademark and licence records are
carried through. Roboto is Apache-2.0 and reserves no font name; its rename is for the match, not the licence.

## Consequences

- Every Markdown conversion's `/BaseFont` changes (`…+NibText-Regular`, `…+NibMono`). The glyphs, widths and
  character maps are upstream's, so layout does not move.
- **ADR-033's label claim was re-measured on the landed change**: `TestNibsOwnMarkdownConversionConforms-
  AcrossEveryConstruct` passes with veraPDF, as it did before the rename.
- A document carrying a foreign `Roboto` keeps nib's watermark and page numbers in an embedded face.
- The co-signing readme is drawn in the new faces. It is rendered once, by the initiating party, and travels as
  bytes; nothing compares a readme rendered by one build with one rendered by another (`RenderReadme` has one
  production caller, `AppendReadme`).
- **Declared:** the OCR text layer's Latin face is still pdfcpu's own bundled `Roboto-Regular`, under that name
  (`ocrFontFor`). It is a different door (pdfcpu installs that face itself), and ADR-101's "a word is Nib's only
  when all of it is the stamp's shape" reads layers older builds wrote — renaming it is its own decision.
- Documents nib already wrote keep their `Roboto` faces; nothing reads a face name to recognise nib's own
  conversion.
