# ADR-129 — a font the validator takes out is put back for the write

**Status:** accepted
**Date:** 2026-10-10
**Extends:** ADR-116 (the checker carries two fonts across pdfcpu's validator) from the checker's read to every
WRITE, by a different mechanism. ADR-009 (one door). ADR-013 is untouched: `ContentDigestVersion` does not move.
**Context:** /pending 842. pdfcpu's relaxed validator does not only judge a document, it changes it. A font it
cannot validate is not refused: `fixFontObjNr` (`validate/font.go:1153-1164`, v0.13.0) sets its entry in the
`/Font` dictionary to nil, or points it at another font of the same `/BaseFont` where the dictionary holds one.
Three shapes reach it: a Type 0 font whose descendant fails validation (`:756-760` — veraPDF's
`7.21.3.2-t01-fail-a.pdf` and `-fail-c.pdf`, `/CIDToGIDMap /NoIdentity`), a font written directly in the
dictionary, and a reference that names no font dictionary (`:1114`, `:1132`). pdfcpu's writer leaves a nil entry
out. So EVERY nib rewrite — a note, a watermark, the ceremony record, a rotation — wrote the page without a
font its content still names, and `ContentDigest`, which counts a resource dictionary's entries
(`hashResourceDict`), moved under rules 4 and 5: convening such a document failed its own check.
**Applies:** every function that serialises a pdfcpu context, in every package.

## Decision

**1. One door writes: `pdfread.Write`.** It puts back what the validator took out, then calls pdfcpu's writer.
No other file names `api.WriteContext`, `api.Write`, `api.WriteIncr(ement)` or `pdfcpu.WriteContext`
(`TestEveryWriteRoutesThroughTheDoor`). Fourteen call sites in `pdfops`, `sign`, `mdpdf`, `testpdf` and `pdfread`
itself went through it.

**2. The entry is put back for the write, and only for the write.** Between the read and the write — the
optimize pass, the operation, every reader, the digest — the context is what it has always been, the
validator's reading. This is what makes it safe where ADR-115 §2 forbids `ReadOptimizedOrRefuseSettingAside` for
a written context: there, an entry nothing vouched for is READ and acted on by rules; here nothing reads it. It
is carried — the reference the file held, to the object the file held, by a writer that serialises and does not
interpret. Not carrying it is the unvouched act: a deletion nobody asked for. Restoring at the read instead
(one line in `validated`) was refused for that reason, and because it would change what the checker and the
digest see.

**3. What was lost is found after the validation, from its marks, and the file is parsed again only where one
shows** (`rememberValidatorLosses`). An emptied entry is nil; a re-pointed one shares its reference with a
sibling. A `/Font` dictionary with neither is as it was read. Where one shows, the bytes are read a second time
unvalidated and the dictionary compared entry by entry at the same object and path, which tells a loss from a
`null` the document wrote and from two names it bound to one font. A snapshot BEFORE the validation was built
first and refused: an object held in an object stream is not decoded until asked for, so the snapshot saw no
font dictionary in any file pdfcpu had written — every second rewrite lost the font again — and decoding them
all to look moves reference counts on documents with nothing to restore.

**4. An entry is put back only where it is still as the validator left it** and its target is still the
cross-reference entry the document named. One the operation removed stays removed; one it rebound keeps the
operation's font.

**5. A page is found again from the page.** pdfcpu's optimize pass gives every page a new `/Resources`
(`optimizeResourceDicts`), so the nil entry sits in a dictionary no read saw; each loss is mapped to the pages
that draw through it (nearest dictionary holding the name) while the dictionaries are still the read's own. A
dictionary on a page-tree node that every page under it now has a copy of is NOT restored: the validator takes
a font out only where it meets it FIRST, so a second reference the document never had would be lost on one path
and kept on the other at the next read, and the digest would move.

**6. A context whose objects are about to move is restored before the move** (`PutBackValidatorLosses`, named
at the site): a merge source (its references are renumbered), `CutPage` (a new context) and `NUpFromPDF`
(resources copied into a form). The guard holds a function that calls one of them to calling it. A moved
context left unrestored is written as before — an entry is never restored into a numbering it did not come from.

## Consequences

- On both corpus files and a hand-built fixture (four fonts: lost Type 0, direct, dangling, plain; and with a
  same-named twin), after a rewrite that changes nothing, `EmbedCeremonyRecord`, `AddNotes`, `StampWatermark`,
  `Rotate` and `Append` either way round, the page names every font it named and each reaches what it reached;
  where the operation leaves content alone the digest is the source's under both rules
  (`TestARewriteKeepsTheFontsTheValidatorTakes`, and three beside it).
- **What such a font hashes as does not change.** The digest reads through the validator and hashes the entry
  as `#nil` before and after this decision; it still does not cover that font's program. Whether it should is
  the owner's question and a `ContentDigestVersion` matter (ADR-013).
- Measured over `~/nib/producers` (36) and `~/nib/verapdfs` (297), 331 read: 2 documents have a `/Font` loss —
  the two named. A no-op rewrite and `AddNotes` of every file, dumped object for object before and after this
  change: identical but for those two documents (the font and what it reaches, 7 objects each) and three tagged
  files whose new annotation takes a different free object number on every run of the UNCHANGED tree as well.
- Cost: one walk of the objects a read already holds — 47 ms summed over 331 validated reads taking 1.73 s
  (2.7%) on a loaded machine — on EVERY validating read, readers included. The second parse is paid only by a
  document with a mark.
- **Declared gaps.** The validator changes more than fonts, and none of it is restored here — counted on the
  same corpus by comparing every object before and after validation: the catalog's `/Outlines` removed (17
  documents), `/AcroForm` removed (10) or rewritten (20), `/Names` removed (3) or rewritten (23), a page's
  `/Annots` rewritten (190 pages, 15 documents), a `/Fields` holder rewritten (5). Which of those lose something
  a user would miss is unmeasured. A font shared by two dictionaries is taken out of the first the validator
  meets and not the second, and pdfcpu ranges a dictionary in map order, so which twin a same-named font is
  pointed at is not fixed either; both are pdfcpu's and predate this. `pdfread.MergeRaw`'s restore is held by the
  guard and by no behaviour test. The checker's own search (`fontsTheValidatorLoses`, ADR-116) walks
  `e.Object` before validation and so has the object-stream blind spot §3 describes — unmeasured there.
- **2026-10-10, the declared gaps measured (/pending 866) — one is a loss, and it is NOT restored here.** Each
  entry of the same 331 documents was read without the validator, before and after a rewrite that changes
  nothing, and compared by what it reaches (references followed, never printed), not object for object.
  Carrying nothing: every `/Outlines` removed (17) had no `/First` (`validate/outlineTree.go:454`); every
  `/AcroForm` removed but one (10 of 11) held `/DA`, `/DR` and an empty `/Fields` (`validate/form.go:744`);
  `/Names` removed (3) held only an `/IDTree`, which is not a name-dictionary key — the same object the structure
  root's own `/IDTree` still names in the two documents where it is not empty; `/Names` changed (5) lost an empty
  `/EmbeddedFiles` (`validate/xReftable.go:316`, `:333`); and no surviving `/AcroForm`, `/Fields` or page
  `/Annots` reaches anything different afterwards — those counts were a direct object made indirect or an array
  built again. **The loss: an XFA-only form.** `form.go:734` and `:744` delete an `/AcroForm` whose `/Fields` is
  absent or empty whatever else it holds, so veraPDF's `7.15-t01-fail-a.pdf` (eight XFA packets) and a hand-built
  fixture of either shape are written with no `/XFA` and with `/NeedsRendering true` still in the catalog.
  **It cannot be put back the way a font is.** The validator deletes the catalog's KEY, so at the write "the
  validator took it" and "the operation took it" are the same catalog, where an emptied `/Font` entry is told
  from a removed one (§4); `StripActive`, the signature strip and `pruneAcroForm` all end by having no form, and
  a put-back would hand `StripActive`'s output the XFA scripts `Scan` reports and it exists to remove. pdfcpu's
  merge also adopts a source's `/AcroForm` (`merge.go:632`), so the pre-move restore of §6 would graft one
  document's XFA onto another. Restoring it needs the form to be SEEN between the read and the write, which §2
  refused for fonts, and is a decision of its own. Two more, read from the source and shown on fixtures, in no
  corpus document: an outline with one empty item dictionary is removed whole, titled bookmarks with it
  (`outlineTree.go:406`, `:483`, `:471`), and a name tree whose only key is the empty string is dropped
  (`xReftable.go:325`).
