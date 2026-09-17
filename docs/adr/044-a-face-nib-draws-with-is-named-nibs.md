# ADR-044 — a face nib draws with is named nib's own

**Status:** accepted

## Context

pdfcpu matches a watermark's font to a font already in the document **by name and by nothing else**
— `stamp.go`'s `createFontResForWM` tests `fo.FontName == wm.FontName && fo.Prefix != ""` — and then
rebuilds that font from nib's TTF using the **document's** glyph ids (`UpdateUserfont`).

Nib stamps with Liberation. LibreOffice writes Liberation. So every edit, signature stamp or page
number nib drew onto a LibreOffice document — which includes nib's own office conversion — hit that
match. The document's own `BAAAAA+LiberationSans` failed the bake with `corrupt fontDict`, and **a
subset with different glyph ids would have been rewritten silently**, which is the worse case
because it is not an error, it is wrong glyphs.

P01.S03 closed the silent case the only way it could from inside: `stampTextWatermarks` refuses any
same-named font nib's face does not describe exactly and draws that stamp in a Base-14 face, logged.
That is correct and it leaves PDF/UA **7.21.4.1** failing on exactly the documents an office suite
produces. Measured through nib's own office door on an RTF conversion: the input fails 7.1 t8 and
7.1 t10, and after `StampFields` it additionally fails **7.21.4.1 t1**, with the refusal in the log.

**The name is the whole of the match**, so the whole of the fix is the name.

## Decision

**Nib installs the faces it draws with under a PostScript name no producer uses** — `Liberation`
becomes `Nib` in the `name` table — so pdfcpu's matcher can never find one of nib's faces already in
a user's document, and nib's own walkers can never reach a stranger's font.

Three things this turns on, each measured rather than assumed:

1. **The name must be in the BYTES.** An in-memory registry alias is not enough: `font.Read` fetches
   the program from `<name>.gob` on disk, so a face aliased only in memory is fetched under the
   upstream name anyway.
2. **The rename is confined to the `name` table**, and to name IDs 1, 3, 4, 6 and 16–25 within it.
   Every other table is byte-identical across all twelve faces — `glyf` 269,352, `hmtx` 10,480,
   `cmap` 1,688 bytes for LiberationSans — and `head` differs only in `checkSumAdjustment`. pdfcpu's
   parsed `TTFLight` compares equal but for `PostscriptName`: 2,620 glyphs, 2,620 widths, 2,327 cmap
   entries, unitsPerEm 2,048, identical bbox.
3. **The vendored bytes are NOT modified.** `internal/pdfops/fonts/*.ttf` are upstream's, byte for
   byte; the rename is applied to a runtime copy on the way into pdfcpu's font directory.

**The licence points the other way from where it first appears to.** Liberation is OFL 1.1 with
Reserved Font Name "Liberation", and pdfcpu already embeds a *modified* copy — `font.Subset` prunes
`glyf` and `loca`. So before this change nib shipped a modified Liberation still called
`LiberationSans`, which is the thing §3 asks you not to do. Renaming is compliance, not a risk to
it. Name IDs 0 (copyright), 7 (trademark) and 13/14 (licence) are carried through unchanged, and a
red proof holds that line.

## Consequences

**An edit on an office document is drawn in a real embedded face again.** Measured: after the
rename, `StampFields` on that same RTF conversion adds **no** clause — 7.1 t8 and 7.1 t10 in, 7.1 t8
and 7.1 t10 out — and the document's own font program is byte-identical across the stamp, 13,468
bytes in and out.

**Nib's by-name xref walkers are now safe by construction.** `dropCIDSetsOf` and `repairToUnicodeOf`
walk by base name; today they are kept off a stranger's font only by never being allowed to run.
After this they can only reach a font nib wrote.

**The alternative is recorded because it works.** Hiding the colliding `ctx.Optimize.FontObjects`
entry from pdfcpu's matcher was built and measured — zero added ua1 clauses, the document's font
untouched — and is the fallback if this is ever unwound. It was not chosen because it leaves those
two walkers matching a name the user's producer also uses.

**The declared gap: the AUTHORING faces are not renamed.** `AuthoredTextFaces` — Roboto in four
weights plus LiberationMono — reaches `stampTextWatermarks` through the same door, so a document
carrying a foreign `Roboto` still drops `StampWatermark` and `StampPageNumbers` to Base-14. It is
out of scope here because renaming them changes every Markdown conversion's `/BaseFont` and touches
ADR-033's PDF/UA label claim, and because LibrationMono carries the same OFL §3 point. It is filed
rather than left to be rediscovered.
