# ADR-117 — the checker reads a Type 0 font with the CMaps pdf.js ships, where they read as veraPDF's do

**Status:** accepted
**Date:** 2026-10-09
**Context:** `/pending 676`. A Type 0 font can lean on two kinds of CMap its document does not hold. Its `/Encoding`
may NAME one of ISO 32000-1's predefined CMaps (`90ms-RKSJ-H`), which decides how its strings are cut into codes and
which CID each code is. And a glyph with no `/ToUnicode` entry has, in veraPDF, the text Adobe's
`Adobe-<ordering>-UCS2` CMap gives its CID — the descendant's ordering under an Identity CMap, the CMap's own
otherwise. nib carried neither, so every such glyph was `CannotCheck`: veraPDF's own 7.21.7 pass file (KozMinPro
over Adobe-Japan1) among them, and with a predefined `/Encoding` the width and glyph-presence clauses too.

The tables were already in the repository: pdf.js ships Adobe's cmap-resources packed as `.bcmap` files
(`web/vendor/pdfjs/cmaps/`, 169 files, 1.7 MB), and their licence is already in the notices.
**Extends:** ADR-052 (shown bytes are read through one door, as veraPDF reads them).
**Applies:** the PDF/UA checker; `internal/fontcode`; the new `internal/cmapres`.

## Decision

**1. The checker carries copies of the tables it needs, and a test holds each copy identical to the vendored file.**
`go:embed` cannot reach a parent directory, `web/` is the root package's, and the checker cannot import the root
package; a package under `web/vendor/` is not importable at all (Go reads `vendor` as its own). So
`internal/cmapres/bcmap/` holds 53 of the files — 347 KB — and `TestEveryCarriedCMapIsTheVendoredFile` fails on a
copy that differs. Nothing is carried that the repository did not already carry.

**2. A carried CMap is written out as a CMap program and read by the readers an embedded CMap is read by.**
`cmapres` decodes pdf.js's packed form (written from the reader in the vendored build; notdef ranges, which that
reader skips, are kept) and writes the PostScript text back out. `fontcode` reads it with `ParseCodespace`,
`ParseCIDMap` and `ParseToUnicode` — so the counted lists, the order mappings answer in, the merge at a `usecmap`
and the cut at a byte no range admits are the rules already measured against veraPDF, not a second copy of them.
`cmapres.write` is the one function outside `fontcode` that spells a CMap list's name, and the routing guard
(`TestTheReadersRouteThroughThisDoor`) names it as a writer.

**3. A table is carried only where it READS AS veraPDF's own does — these are not the same files.** veraPDF answers
from the copy of Adobe's CMaps in its jar; pdf.js packs another version. `TestEveryCarriedCMapReadsAsVeraPDFsOwnDoes`
reads each carried CMap beside veraPDF's file of the same name, with the same reader, wherever veraPDF is installed.
Measured on 1.30.2:

- **49 of the 59 predefined CMaps that are not Identity agree everywhere**: the same codespace ranges in the same
  order, the same CMap used, and the same CID, held or not, at every code where either file's mappings begin or end.
- **Ten do not, and are not carried**: `GBK2K-H`, `UniCNS-UTF16-H`, `UniGB-UTF16-H`, `UniJIS-UTF16-H`,
  `UniJIS-UTF16-V`, `UniKS-UTF16-H` differ in 3 to 217 places (veraPDF's files are newer supplements), and
  `GBK2K-V`, `UniCNS-UTF16-V`, `UniGB-UTF16-V`, `UniKS-UTF16-V` use one of them. A font over one refuses, by name.
- **The four UCS2 CMaps give 1,769 CIDs different TEXT** (Japan1 1,607, CNS1 133, GB1 28, Korea1 1) — a figure dash
  for an en dash, a character with and without a variation selector. 7.21.7 does not read the text: t1 asks whether
  a glyph has any, t2 whether it holds U+0000, U+FEFF or U+FFFE. On those two questions the versions part at 95
  CIDs, in three runs — Adobe-CNS1 19088–19178, Adobe-Japan1 23053–23054 and 23058–23059 — which `fontcode.ucs2Gaps`
  lists and the test holds exact in both directions. A glyph whose CID is in a run is `CannotCheck`.
- **`Adobe-KR-UCS2` is not among pdf.js's tables.** An Adobe-KR font with no `/ToUnicode` entry refuses, as before.

**4. The UCS2 CMap is asked the glyph's CID, not its code.** Measured: under `90ms-RKSJ-H` the code `82A0` passes
7.21.7 t1 — it is CID 843 — and as a CID of its own it is past the collection's end. The code goes through the
font's CMap chain first (`cidMapOf`), charged to the document's CMap budget like the width rules' lookups.

**5. A `usecmap` naming a carried CMap is merged where the operator stands**, in an embedded CMap's program as in a
carried one's (24 of the carried CMaps use another, most a vertical sibling using the horizontal one): its codespace by
`Codespace.Merge`, its cid mappings behind the program's own, its notdef mappings at that point — once per name,
since a second copy can answer nothing the first has not and a program repeating the operator would copy the table
each time. A carried CMap is read once for the process and shared; nothing writes to it afterwards.

## Refused

- **Carrying veraPDF's own versions of the files.** It would make the ten CMaps, Adobe-KR and the 95 CIDs
  answerable, for 3.4 MB of new data (the 64 files as they stand in veraPDF's jar, measured) whose only purpose is to
  track one validator's bundled copy. Open if the refusals bite.
- **Merging a carried UCS2 CMap into a `/ToUnicode` that `usecmap`s it.** There the merged entry's own text is what
  the font answers with next to its own entries; that shape still refuses.
- **Moving the tables out of `web/vendor/pdfjs/`** so one copy serves both: the window's pdf.js fetches them by URL
  from that directory, and the notices and the vendoring guards name it.

## Consequences

- 66 documents built for it answer every clause as veraPDF does or refuse with a reason
  (`TestAType0FontIsReadWithTheCMapsNibCarries`); veraPDF's corpus file `7.21.7-t01-pass-a.pdf` passes both tests.
  The oracle's 18 pinned refusals over these shapes are gone, and the corpus reach of five clauses rose to 285.
- Two more corpus files have every clause nib checks passing (142, from 140), which the README's figure states.
- The binary grows by the 347 KB of copies.
- **A long document over a predefined CMap can still refuse, on cost.** A lookup walks the CMap's mappings in
  veraPDF's order, per distinct code, and is charged to `maxCIDAsks` (4,194,304) as an embedded CMap's is; the
  largest carried CMap holds 16,432 mappings (`UniCNS-UCS2-V`), so a few hundred distinct codes that sit deep in it
  spend the budget. Not measured on a real document. An index over a carried CMap — read once, for the process —
  would remove it, and is not built here.
- A pdf.js update that changes a table fails the identity test until the copy is refreshed — and then the
  comparison with veraPDF's files decides again which tables are carried and which CIDs are gaps.
