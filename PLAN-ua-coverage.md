# PLAN — PDF/UA coverage: what nib's own edits break, and the checker's other 87 rules

**Dateline.** Seeded 2026-09-15 from `/grill "both option C and tagging"`, approved by Dan the same day.
Every figure below was produced by running veraPDF on 2026-09-15 at v1.129.112, not by reading code.

**Where this plan and the request differ, the plan wins.** The request was *"full tagging of annotations"*
beside option C. Measured, annotations are the smallest part of what nib's own edits break: one operation
breaks annotation rules, while eight page-set operations drop the whole structure tree and three stamping
operations draw in fonts they do not embed. The writing track is built against what was measured.

**Status: P01 closed** (v1.129.119), **P02 closed** (v1.138.16), **P03 closed** (v1.144.1, the checker at 58 of 106). P04 (language) is opened: four slices, twelve rules. There is no P00 — nib needs no bootstrap.

---

## What is already true, and was measured rather than assumed

- **veraPDF's PDF/UA-1 profile is 106 rules; nib's checker implements 19** (`internal/uacheck`, the
  `Clause:` registry). The 87 it lacks, with veraPDF's own descriptions, come from
  `verapdf --flavour ua1 --passed` on a conformant file.
- **veraPDF's own corpus covers 97 rule ids** (297 files under `~/nib/verapdfs/PDF_UA-1`), and
  `internal/uacheck/veracorpus_test.go` already scores nib against it: 0 false pass, 0 false fail for the
  19. About 9 of the 87 have no corpus file.
- **The 87, by what they need to read:** ~40 structure-tree containment and role rules (tables, lists, TOC,
  headings, notes, Form/Link, role maps); ~10 language rules (outlines, ActualText/Alt/E, annotation
  Contents, form TU, marked-content spans); ~9 annotation rules; ~11 file-level rules (identification prefix,
  header, Suspects, embedded-file keys, XFA, encryption P, reference XObjects, Form XObject content, Formula);
  ~12 font-program rules (CMaps, CIDToGIDMap, width agreement, TrueType cmap/encoding, CharSet, `.notdef`,
  ToUnicode values).
- **`uacheck` already reads** content events, appearance streams, the ParentTree, structure nodes, table
  attributes, fonts (used fonts, descriptors, a TrueType `maxp` glyph count) and the XMP packet.
- **What nib's own operations break**, each driven on a veraPDF-conformant labelled Markdown conversion
  (5 t1 ignored — ADR-032 fails it on purpose):

  | Operations | Clauses added |
  |---|---|
  | `AddNotes` | 7.18.1 t1, 7.18.3 t1 |
  | `AuthorTaggedForm` (the app's form door) | 7.1 t8, 7.21.4.1 t1 |
  | `Collect`, `Crop`, `DuplicatePage`, `Booklet`, `SplitPage`, `SplitRegions` | 6.2 t1, 7.1 t3, t8, t10, t11 |
  | `InsertPDF`, `CarryAttachments` | the same, plus fonts and language |
  | `StampFields`, `StampPageNumbers`, `StampWatermark` | 7.21.4.1 t1 |
  | `Append`, `Combine` | 7.1 t3, 7.21.4.1 t1 |
  | `StripMetadata` | 7.1 t8 (by design: it removes the metadata) |

  Fifteen operations add nothing, among them `Rotate`, `Optimize`, `NUp`, `SetOutline`, the labels, flags,
  attachments and the OCR text layer.
- **Widgets are already tagged the right way.** `tagform.go` nests each in a `/Form` element through
  `/StructParent` and an `OBJR`, which is the shape an `/Annot` element takes.

## The laws this plan keeps

1. **A checker rule is correct only where veraPDF agrees with it** — on the corpus, and on real producers'
   files. A rule nib cannot evaluate says `CannotCheck` and names why; it never says `Pass`.
2. **106 rules is "agrees with veraPDF", never "PDF/UA conformant".** PDF/UA has human checkpoints no machine
   reads. Nothing in the docs or the report may say more.
3. **A nib operation adds no PDF/UA failure to a conformant document** unless the loss is declared by name
   (ADR-031's visible-loss rule applies where carrying is impossible).

---

## Decisions

### D1 — The writing track goes first *(settled 2026-09-15 via /grill — default, Dan may reorder)*
It makes the documents nib hands users better now; the checker track makes the report more complete. Neither
depends on the other.

### D2 — One writing-side census is the reader for every writing phase *(settled 2026-09-15 via /grill)*
Every driven operation, on a conformant document, compared clause by clause against the document it was
given. Built first, because every later phase proves itself by moving a row of it. Skips as UNMEASURED where
veraPDF is absent, like every veraPDF test in the tree.

### D3 — Notes become `/Annot` elements through the widget door's shape *(settled 2026-09-15 via /grill)*
`/StructParent` (singular) on the annotation, an `OBJR` in an `/Annot` element, `/Contents` carried, `/Tabs S`
on the page. ADR-009: the ParentTree bookkeeping `tagform.go` already does is not written twice.

### D4 — Stamped text is drawn in an embedded face *(settled 2026-09-15 via /grill)*
The OCR layer already stamps in embedded faces and fills nothing, so the pdfcpu fill defect that keeps
`/pending 479` deferred does not apply to stamps.

### D5 — Page-set operations carry the tree for the pages they keep, or drop the claim *(settled 2026-09-15 via /grill)*
`NUp`'s carry (`carryTagsThroughNUp`) is the precedent; `honest` stays the backstop. A remap that cannot anchor
every element drops the claim rather than ship a partial tree.

### D6 — The checker is Go, not a veraPDF delegation *(settled 2026-09-15 via /grill — assumed, Dan may override)*
Delegating to veraPDF when Java is installed was considered and refused: nib would answer differently on
machines with and without Java, and the small-practice user has no Java.

### D7 — A real-producer corpus validates the checker, not only veraPDF's synthetic one *(settled 2026-09-15 via /grill)*
The synthetic corpus holds one construct per file; LibreOffice, Word, Acrobat and Ghostscript output is where
the font family diverges. Without it, a rule agreeing on the corpus is a claim about the corpus.

### D8 — Reaching 106 rules does not change who gets labelled *(settled 2026-09-15 via /grill — default)*
ADR-033 stands. Widening the label is a decision for Dan after the checker track closes.

---

## Build order

### P01 — The writing-side census, and the three cheap losses *(done 2026-09-16, v1.129.119 — one criterion clause open, named below)*
**Goal.** A standing measurement of what every nib operation does to a conformant document, and the fixes for
the losses that are not a structure carry: notes, stamped text and the form door's metadata.

**Exit criteria.**
- A census drives every operation that has a tag-fate drive and fails on any ua1 clause added that is not
  declared.
- `AddNotes`, the three stamping operations and `AuthorTaggedForm` add no clause.
- The page-set, merge and `StripMetadata` losses are declared by name in the census, each pointing at its phase
  or its reason.

**(phase close, 2026-09-16, v1.129.119)** Acceptance ledger, clause by clause:
- [x] a census drives every operation that has a tag-fate drive — `ua1oracle_test.go:198`; a job with no output fails (`:246`).
- [x] and fails on any ua1 clause added that is not declared — both directions (`:286`); probed red at S01 and at S03.
- [x] `AddNotes` adds no clause — row removed, census green (S02).
- [x] the three stamping operations add no clause — rows removed, census green (S03).
- [ ] **`AuthorTaggedForm` adds no clause — NOT MET, parked:** 7.1 t8 is gone (S04), 7.21.4.1 t1 stays declared against
  `/pending 479`, deferred on pdfcpu filling a field set in an embedded face (S04's own acceptance kept it).
- [x] page-set losses declared by name, each pointing at its phase — `pageSetLoss` rows name P02.
- [x] merge losses declared — `Append`/`Combine` rows name ADR-031's recorded `partial`.
- [x] `StripMetadata`'s loss declared with its reason — permanent, by design.

Required-run gates: tiers 0–3 at every slice close and at v1.129.119 (tier 3: `/pending 474`'s three known reds only).
Tier 4 and tier 6 **did not fire**: no P01 slice touched `internal/server`'s session, ceremony, delivery or discovery
paths, `internal/p2p` or `internal/rendezvous`.

Full-repo review: `code-reviews/v1.129.118-2026-09-16.md` — the three findings P01 introduced or falsified are fixed
(a stamped face past 100 glyphs, filling a form dropping `/Metadata`, the OCR door's unscoped `/CIDSet` drop); seven
criticals and the rest are pre-existing and filed as `/pending 495`–`506`. Closure sweep: no pending item was gated on
P01; `/pending 319` closed by the review's sweep.

#### P01.S01 — the census *(done 2026-09-15, v1.129.114)*
Scope: a veraPDF test over the tag-fate population on a labelled Markdown conversion, with a declared-losses
table. Refs: D2.
Acceptance:
- An undeclared added clause fails, naming the operation and the clause.
- A declaration whose loss no longer happens fails too, so the table cannot outlive the defect.
- No veraPDF: skips as UNMEASURED.
Tasks: *(written at slice-grill time, 2026-09-15; T01–T02 re-cut by the slice review the same day)*
1. T01 — ~~`internal/pdfops/uacensus_test.go`: a new census~~ **Rebase the census that already exists**,
   `TestNoOperationAddsAUA1ClauseItsInputDidNotFail` (`ua1oracle_test.go`, `PLAN-accessibility.md` P03.S03), onto
   a labelled conversion of several pages, and delete the duplicate the slice first built.
2. T02 — merge the two declaration tables into `knownUA1Deltas`: one row per operation that loses a clause
   today, each naming P01.S02–S04, P02, `/pending 479`, ADR-031's `partial`, or a permanent reason. 5 t1 is
   excluded for all (ADR-032). `knownUnvalidatable` empties (`RemovePages` validates on several pages).
3. T03 — the stimulus floor flips from "the fixture fails something" to "the document is conformant", keep the
   missing-job and both-ways checks, then probe each direction red.

**(review pin, 2026-09-15, P01.S01)** **D2's premise was wrong: the census existed.** The slice built a second one
beside `TestNoOperationAddsAUA1ClauseItsInputDidNotFail`, which already drove `tagFates` through one veraPDF batch
with a both-ways table — and "What is already true" above does not mention it. The slice review
(`code-reviews/v1.129.113-2026-09-15.md`) caught the duplicate. **The existing census had a resolution gap, and
that is what the new one had found:** its one-page hand-built fixture already failed 7.1 t8, 7.1 t10 and
7.21.4.1 t1, so every operation adding one of those was invisible to it. The decision holds — one census is every
writing phase's reader — and the slice became a rebase and a merge.

#### P01.S02 — notes are `/Annot` elements *(done 2026-09-16, v1.129.115)*
Scope: `AddNotes` on a tagged document nests each note, sets `/Contents` and `/Tabs S`. Refs: D3.
Acceptance:
- The census row for `AddNotes` adds nothing.
- An untagged document gains no tree from a note (ADR-031: no claim over nothing).
Tasks: *(written at slice-grill time, 2026-09-16; deep-dive: one caller, `handleBake`, notes last)*
1. T01 — generalise `tagWidgetsOnPage` to `describeAnnotationsOnPage(subtype, elemType)`: the widget door
   and the note door are the same three writes (ADR-009).
2. T02 — `setStructureTabOrder(ctx)` so `/Tabs /S` rides `AddNotes`'s own rewrite; the form door calls it too
   (inside its own rewrite since P01.S04).
3. T03 — `AddNotes` describes each note as an `/Annot` only when the catalog already has a `/StructTreeRoot`;
   `/Tabs /S` on every annotated page either way.
4. T04 — drop `AddNotes` from `knownUA1Deltas`; a writes test and an untagged-document test.
5. T05 — *(from the slice review, `code-reviews/v1.129.114-2026-09-16.md`)* describe notes in page order, and in a
   second pass that returns the notes undescribed when nib cannot model the tree, so a note never fails the bake.

**(review pin, 2026-09-16, P01.S02)** **T03 as written made a note able to fail a save.** Describing inside
`AddNotes`'s one rewrite returned the parser's refusal of a third-party tree as the operation's error — `handleBake`
answers 500 and the client aborts the save. `AuthorTaggedForm` already had the right shape (never cost the caller the
form), and the notes door now has it too. The only such tree reachable through pdfcpu's relaxed read is an element
with no `/S`: pdfcpu refuses a bare MCID or MCR under the root, and its structure depth limit (100) fires before nib's
(200).

#### P01.S03 — stamped text embeds its face *(done 2026-09-16, v1.129.116)*
Scope: `StampFields`, `StampPageNumbers`, `StampWatermark` draw in the authoring face. Refs: D4.
Acceptance:
- Their census rows add nothing.
- A Base-14 fallback when faces cannot install is still reported, not silent.
Tasks: *(written at slice-grill time, 2026-09-16; deep-dive not fired — the seam is `resolveFit`'s measurement, read
in full at `pdfops.go` 1136–1411, and `mdpdf`'s width/wrap doors, read at `layout.go` 22–126 and 403–417)*
1. T01 — vendor Liberation Sans and Serif (four styles each) and Liberation Mono's other three, with their notice.
   **Not the authoring face for `StampFields`, and the scope line above is amended by measurement:** a field's face
   is chosen to MATCH the document run it replaces (`classifyFont`), and the fit verdict, the shrink and the wrap are
   all measured in that core face. Liberation is metric-compatible with all twelve core text faces — measured within
   0.21% of Helvetica and Times at 12pt, identical to Courier — so the embedded draw keeps every measurement and the
   client's preview. Roboto has no serif and moves the widths. *Assumed:* ~4.1 MB of binary for it.
2. T02 — `stampFacesInstalled()` and `stampFaceFor`: the core→Liberation map, installed all-or-nothing through
   `mdpdf.InstallFaces`; on failure log and draw the core names.
3. T03 — `StampFields` draws in the mapped face and measures with the embedded rule (`mdpdf.Width`); wrapping gains
   an embedded-aware `mdpdf.Wrap` that `WrapCore` delegates to (ADR-009).
4. T04 — `StampWatermark` and `StampPageNumbers` draw in `AuthoredTextFaces`' body face.
5. T05 — drop the three census rows; a test reads the output's fonts for all twelve core names, and one proves the
   fallback reports.
6. T06 — *(from the census)* the stamps route through `embeddedFontsAreHonest`'s rule: pdfcpu's used-glyph `/CIDSet`
   on a Liberation subset (glyphs 0 and 87, set names 87) failed 7.21.4.2 t2.
7. T07 — *(from the slice review, `code-reviews/v1.129.115-2026-09-16.md`)* one read-change-write per stamp
   (`stampTextWatermarks`), which draws embedded only when a same-named font already in the document is one nib's
   face describes exactly, and retries in Base-14 on an embedded failure.

**(review pin, 2026-09-16, P01.S03)** **pdfcpu reuses a document's font by NAME and rebuilds it from nib's TTF.** A
LibreOffice document's own `LiberationSans` failed the bake outright (`corrupt fontDict`), and a subset with other
glyph ids would have been rewritten silently. Such a document now gets its stamps in Base-14, logged — so 7.21.4.1
still fails for edits on documents that carry their own Liberation or Roboto, which LibreOffice output commonly
does. Recorded as a pending item.

#### P01.S04 — the form door keeps the metadata *(done 2026-09-16, v1.129.117)*
Scope: `AuthorTaggedForm` carries the catalog `/Metadata`. Refs: —
Acceptance:
- Its census row adds no 7.1 t8.
- 7.21.4.1 stays declared against `/pending 479`.
Tasks: *(written at slice-grill time, 2026-09-16; deep-dive not fired — one function, `authorFormIn`, whose three
callers are the two form doors and `/pending 479`'s gate test)*
1. T01 — measured first: `api.Create` over an existing document drops exactly one catalog key, `/Metadata`;
   `create.FromJSON` inside `rewriteWithConf` keeps it. `authorFormIn` creates inside nib's rewrite, and the tab
   order rides the same pass (`setWidgetTabOrder`'s second rewrite goes). **The cause, found by the slice review and
   re-read:** pdfcpu's post-process `ValidateContext`, on by default, deletes the catalog `/Metadata`
   (`validate/metaData.go`); the door keeps it by not validating after creating, as no other nib rewrite does.
2. T02 — both form doors' census rows drop 7.1 t8; a test reads the catalog `/Metadata` back from both doors and
   that the kept packet carries no PDF/UA identification (ADR-032).

### P02 — Page-set operations carry the structure *(done 2026-09-22, v1.138.16)*
**Goal.** `Collect`, `Crop`, `DuplicatePage`, `Booklet`, `SplitPage`, `SplitRegions`, `InsertPDF` and
`CarryAttachments` keep the structure of the pages they keep. Refs: D5.

**Exit criteria.** Each census row adds nothing, or its remaining loss is declared with the reason a carry is
impossible; the tag-fate table's verdicts move from `dropped` to what is measured.

**(phase close, 2026-09-22, v1.138.16)** Acceptance ledger, clause by clause (the goal's operations included, since
the criteria are stated over them):
- [x] **each census row adds nothing** — `Collect`, `RemovePages`, `DuplicatePage`, `Booklet`, `Crop`, `NUp`,
  `CarryAttachments` carry no `knownUA1Deltas` row; `TestNoOperationAddsAUA1ClauseItsInputDidNotFail` green.
- [x] **or its remaining loss is declared with the reason a carry is impossible** — `SplitPage`/`SplitRegions`
  (`7.1 t3`, `7.4.2 t1`: a tile carries no subtree, ADR-049), `RemovePages` (`7.4.2 t1`: the drive removes the
  heading's page), `InsertPDF`/`Append`/`Combine` (`7.1 t3`, `7.21.4.1 t1`: the second document is untagged
  Base-14, ADR-031's `partial`) — `ua1oracle_test.go:122-144`, each with its reason.
- [x] **the tag-fate verdicts move from `dropped` to what is measured** — `tagtable_test.go:177-240`: every subset
  op and `Crop` `carried`, merges `partial`, split tiles `dropped` by decision (ADR-049);
  `TestEveryDeclaredFateIsTheMEASUREDFate` grades each declared verdict against the measured one.
- [x] goal — the pages each operation KEEPS keep their structure: subsets (S04b), Crop (S05, ADR-047), split's
  other pages (S06/S07b, B4), InsertPDF with the original as host (S07b, ADR-048), nib's own pages (S09, ADR-050).
- [x] goal — `CarryAttachments` never touches structure: `carried` on a tagged input (`tagtable_test.go:210`).

Full-repo review: `code-reviews/v1.138.15-p02-phase-close-2026-09-22.md` — nine findings P02 introduced, eight fixed
and one filed (/pending 604, UA-2 only); a re-review of the fix diff found two more, both fixed (an exponential
class-map comparison, and an unplaceable graft turned into a hard error). Pre-existing: four trivial ones fixed
(a `doc.sig` read race, a vault nonce panic, an uncapped split grid, a stale comment), 33 filed as /pending 578-611,
among them Dan's decided `ContentDigest` bump (578). Graduation pass: 123 rows, 114 mechanical keep-live, 9 judged,
0 hot-path; `splice`'s gate stimulus gap DISCHARGED.

Required-run gates, measured at v1.138.16: tiers 0–2 green (tier 1 re-run green after its first run caught three
guards this pass had moved — the `splice` routing guard, three red-proof patches, and a slice-gate predicate that
missed the phrase "Tiers 0-3, 4 and 6"); `-race` over `internal/server` and `internal/pdfops` clean of races; tier 3
**0 failures**; **tier 4** green over both transports; **tier 6** 28/28; **tier 4d** (`-n 4`) a 4-party relay
completed over both transports. All fire: S09 and this close touched `internal/p2p` and `internal/server`.

**(census pin, 2026-09-15, P01.S01 — CORRECTED at phase-open, 2026-09-16)** ~~veraPDF does not accept a tree
re-anchored into Form XObjects~~. **Measured by the phase-open deep-dive
(`deepdives/2026-09-15-page-set-operations-and-the-structure-tree.md`):** it does. 7.20 t2 is veraPDF's
*"Form XObject contains MCIDs and is referenced more than once"*, and the census document has 8 pages but 4
distinct contents, so pdfcpu's optimize merges equal forms and one XObject is drawn twice. On distinct pages
`NUp`'s carry adds only 5 t1 (ADR-032's exclusion). The carry itself re-merges them (its optimizing read), and
then binds one XObject to two sources and overwrites `/StructParents` — a partial carry `honest` cannot see.

**(phase-open, 2026-09-16)** What the dive traced, and what re-cut the sketch:
- **Subset** (`Collect`, `RemovePages`, `DuplicatePage`): pages keep `/StructParents` and MCIDs; the catalog's
  `/StructTreeRoot`, `/MarkInfo`, `/Metadata`, `/ViewerPreferences` are dropped. A carry by pruning the source
  tree was measured `carried` and veraPDF-compliant, reorders included.
- **Booklet** is `InsertBlank → Collect → NUp`: it drops only because `Collect` does. It needs no slice.
- ~~**InsertPDF** is `Collect + Append` — subset and merge, not composition.~~ **Struck at S04b, measured:** the merge IS the composition. `api.MergeRaw` keeps only the first document's catalog and `/ParentTree`, so a carried left segment plus a tagged inserted document gives pages that hold one document's content and resolve to the other's elements. `splice` takes the non-carrying door; **P02.S07 owns it**, not S04.
- **CarryAttachments** never touches structure; its census row measures the untagged fixture, not the operation.
- **Nothing today can see a partial carry:** `orphaned()` fires only when nothing is anchored.
- **`RedactPages` builds its runs through `Collect`**, so a subset carry would carry a tree over redacted content
  unless redaction explicitly refuses it first.
- nib's census and construct fixtures hold no MCR, OBJR, annotation or RoleMap — each slice below that meets one
  owes its own fixture.

#### P02.S01 — a carry is complete, or it is not a carry *(done 2026-09-16, v1.129.135)*
Scope: a completeness predicate beside `orphaned` (`tagfate.go:124`) — no dead element, MCR or OBJR `/Pg`; every
ParentTree key owned by a live page, XObject or annotation; `checkStructConsistency` empty; no MCID-bearing
XObject drawn twice. **The `NUp` routing moves to S02** (grill, 2026-09-16 — see the measurement below). Refs: D5.
Acceptance:
- The identical-pages `NUp` output and a twin-element/MCR fixture fail it; distinct-pages `NUp` passes.
- A correct producer tree with a key owned by a Form XObject passes it.
Tasks:
- T01 — `structureCarriedCompletely(ctx, tree) []structDefect`: the four conditions in one door, reusing
  `checkStructConsistency` and `parentTreeEntries`.
- T02 — the owner walk gains a form XObject's `/StructParents` (plural), the residue `allocParentTreeKey`
  declares unwalked; one walk shared with the allocator.
- T03 — draws per XObject counted by tokenizing each page's content (`contentstream`), never a byte scan.
- T04 — an MCR fixture and a doubly-drawn-form fixture committed, each probed red against its own condition.
- T05 — the predicate is a reader this slice only; `NUp` keeps its route until S02.

**(grill, 2026-09-16 — MEASURED at the grill, v1.129.134, not reasoned.)** Through `NUp(2)`: the corpus fixture
is clean on all four conditions (and its one ParentTree key is owned by a form XObject's `/StructParents`, which
is the passing clause above); the census has **4 of 8 keys owned by nobody and 2 MCID-bearing XObjects drawn 3
times each**; a hand-built MCR fixture leaves **1 dead MCR `/Pg`**; the twin fixture leaves **1 dead element
`/Pg`**. All four report `carried` today, which is the defect. **Routing `NUp` through the gate in THIS slice
would flip the census n-up to `dropped`** — reddening `TestNoOperationAddsAUA1ClauseItsInputDidNotFail` in both
directions (the `7.20 t2` row goes stale, five `pageSetLoss` clauses appear) and regressing n-up wherever pdfcpu
merges equal forms. The corpus fixture passes the gate, so the tier-1 fate guards are indifferent.

#### P02.S02 — the n-up carry, repaired *(done 2026-09-16, v1.129.136)*
Scope: `tagcarry.go` — visited set by object number, MCR/OBJR `/Pg` repointed and counted, and each placement
given its OWN form XObject so no object is drawn under two semantic parents. **`NUp` is routed through S01's
completeness predicate HERE**, in the commit that repairs the carry, so the census row and the both-ways check
move together (grill of S01, 2026-09-16). Refs: D5, `/pending 503`'s two tagcarry defects.
Acceptance:
- The census `NUp` row disappears (no 7.20 t2), landing with the carry fix so the both-ways check stays green.
- A real tagged PDF keeps every element through `NUp` (LibreOffice-gated, as the corpus recipe is uncommitted).
Tasks:
- T01 — un-fuse: a placement whose form object an earlier placement already anchored gets a cloned dictionary,
  a new xref slot, and that sheet's resource entry repointed at it.
- T02 — the walk's visited set keys on the OBJECT NUMBER, not `d.String()`.
- T03 — an `MCR` or `OBJR` kid's own `/Pg` is repointed and counted, not skipped.
- T04 — `NUp` routes through `structureCarriedCompletely`; a carry that is not complete is abandoned to `honest`.
- T05 — standing readers: an identical-pages `NUp`, the MCR and twin fixtures, and `Booklet` (which is
  `InsertBlank → Collect → NUp` and inherits the whole defect — measured: 8 forms).
- T06 — the `knownUA1Deltas` row `"NUp": {"7.20 t2"}` is removed in this commit; the table is checked both ways.

**(grill, 2026-09-16 — the plan's fourth bullet is REFUTED by measurement.)** "A non-optimizing read so equal
forms stay distinct" cannot work: pdfcpu fuses byte-equal form XObjects inside `optimizeFontAndImages`, which
`OptimizeXRefTable` calls **ungated** and **silently** (the "redundant xobject" logging is on the image path).
`OptimizeResourceDicts=false` changes nothing, and the optimizing reads are several —`rewriteContext`
(`scan.go:482`) and `dropUAIdentificationBytes` (`uaid.go:290`) each have their own, and `withoutUAClaim` runs
BEFORE the carry is called. Caught in the act: `OptimizeXRefTable` alone on a plainly-read context rewrote
`Fm4->416` to `Fm4->424`. A fused form cannot be anchored at all — `/StructParents` is one key on one object and
a form drawn on three sheets needs three — so the carry un-fuses what the optimizer fused. **Measured on the
census through `NUp(2)`:** 4 of 8 keys owned by nobody and 2 forms drawn 3x each → 8 keys each owned by its own
XObject; veraPDF `7.20 t2` + `5 t1` → **`5 t1` alone** (ADR-032's deliberate failure); +4% bytes
(102,072 → 106,118). Radius: `SplitPage` and `Crop` produce no forms; `Booklet` produces 8.

#### P02.S03 — redaction does not carry *(done 2026-09-16, v1.129.137)*
Scope: `RedactPages` explicitly refuses a structure carry, before any subset carry exists. Refs: the census's
recorded RedactPages decision.
Acceptance:
- A redacted document carries no element from the source tree, measured, with `Collect` carrying.
Tasks:
- T01 — `collectWithoutStructure`: today's `Collect` body under a name that will never gain the carry.
- T02 — `RedactPages` binds to it; the two are identical until S04, so the binding is the deliverable.
- T03 — an AST routing guard (`TestRedactionNeverRoutesThroughTheCarryingCollect`), the house idiom from
  `TestTheReviewDoorsRouteThroughTheProposer`: redaction calls the non-carrying door and never `Collect`.
- T04 — the door is not a stub: it still selects pages and still carries `/Lang`.
- T05 — the output-level assertion ships as a declared GAP, not as coverage.

**(grill, 2026-09-16.)** The acceptance clause says *"with `Collect` carrying"* and `Collect` does not carry until
S04 — so an assertion over redacted BYTES is **vacuous today**: `api.Collect` drops the tree on its own, so "no
element survives" is true whatever `RedactPages` calls, and would stay true if redaction were pointed back at the
carrying primitive. The discriminating reader is therefore the CALL GRAPH, and the byte-level reader is recorded in the
phase inventory's S03 section as a backstop that cannot yet fail. **It is not declared to the inventory gate**, whose
`## Known gaps` mechanism excuses a slice with no section and says nothing about a row that exists but is inert; the
obligation sits with S04, which must probe it red against a carrying `Collect`. This is the same failure shape as S02's abandoned-
carry test, which passed with its gate disabled because its fixture was orphaned — asserted properties must be
able to fail for the reason they name.

**(re-cut 2026-09-16, before building.)** S04 was one slice — *"prune the source tree in place and carry the
catalog keys"* — and the investigation found that the prune it names is **not an addition to `Collect` but a
replacement of it**, which is a different and much larger change than the phrase suggests. Three measured facts
forced the split:

- **A tree cannot be transplanted across `api.Collect`.** That path builds a NEW context (`ExtractPages` →
  `CreateContextWithXRefTable`), so every object number changes. pdfcpu's own `migrateObject`/`migrateIndRef` are
  **unexported**, and a hand-rolled deep copy would silently mis-point MCR `/Stm` and OBJR references in real
  producer documents, because nib cannot see pdfcpu's remapping. Nib's fixtures have neither, so the failure would
  not show here.
- **So the prune must run in the SOURCE context**, where object numbers are stable and the tree work is local —
  which means nib takes over page selection from pdfcpu, and inherits `/Dests` migration, AcroForm fields whose
  widgets were on dropped pages, and repeats needing genuine page-object clones (two `/Kids` entries naming one
  page object IS the shared-key defect condition 5 now catches).
- **It must NOT flatten the page tree**, which the phase-open dive's probe did. Measured on a hand-built nested
  fixture read plainly: pages genuinely inherit (`ownResources=false`, `ownMediaBox=false`). Keeping the `Pages`
  hierarchy and only rewriting `/Kids` preserves inheritance by construction; flattening makes nib responsible for
  resolving it. The dive's probe passed only because the census has no inheritance, no dests, no AcroForm and no
  nested tree — the population that cannot exercise any of these risks.

Also governing, and already recorded at `pdfops.go:225`: `/Outlines` and `/PageLabels` are **deliberately not
carried**, because they are page-indexed and copying them across a reorder *"sends the reader to the wrong place —
worse than not having one, because it is wrong rather than absent"*. An in-place prune would start keeping them by
construction, so it must drop them explicitly. (Measured: `Collect` and `RemovePages` drop `/PageLabels` today.)

#### P02.S04a — Collect owns its own page selection *(done 2026-09-16, v1.129.141)*
Scope: replace `api.Collect`/`api.RemovePages` with an in-place page-tree rewrite in the source context — the
ordered, repeat-preserving selection `PagesForPageCollection` produces; ~~the `Pages` hierarchy kept intact so
inherited attributes survive~~ **(struck by the grill, 2026-09-16 — see the pin below)**; a repeated page cloned as
a real page object; `/Dests` naming dropped pages pruned; `/Outlines` and `/PageLabels` dropped explicitly (the
decision above); AcroForm fields whose widgets are gone. **No structure-tree work at all.** Refs: D5.

**(grill pin, 2026-09-16 — the premise "keep the hierarchy so inheritance survives by construction" is FALSE,
measured.)** Two facts settle it. First, the client sends a whole-document permutation (`web/app.js:6036`), and a
cross-subtree reorder MOVES a page to a different parent — at which point what it inherits changes, which is the
exact thing hierarchy-preservation was adopted to protect. Second, keeping the hierarchy is not parity anyway:
`api.Collect` already materializes `/Resources`, `/MediaBox` and `/Rotate` onto every page and emits a FLAT tree
(`pkg/pdfcpu/page.go:137-146`). So this slice emits a flat `/Pages` with all **four** inherited attributes
materialized totally — `InheritedPageAttrs` is exactly `{Resources, MediaBox, CropBox, Rotate}`
(`model/xreftable.go:1698-1704`). Measured bonus: pdfcpu never materializes `/CropBox`, so an inherited crop box is
LOST by today's `Collect` (probe: effective crop width 380 → nil); materializing all four fixes that in passing.

**(grill pin, 2026-09-16 — the drop list is an ALLOWLIST, because in-place inverts a whitelist into a blacklist.)**
`api.Collect` builds a fresh context, so only what pdfcpu migrates survives; rewriting in place means everything
survives unless explicitly dropped, and the plan's four-item list is ~14 items. Measured, today's catalog after a
subset is exactly `{Type, Pages, Names}` (+ `/Lang`, + `/AcroForm` where a field survives), and the source `/Info`
— including `NibFlags` — does NOT survive. So the catalog and trailer are REBUILT from an allowlist rather than
pruned, and an unenumerated key fails safe. The one that is not merely hygiene: a surviving `/StructTreeRoot` is
written deeply and its elements' `/Pg` re-anchors a DROPPED page dict and its `/Contents`, so "delete page 4" would
ship page 4's text. `/MarkInfo` drops WITH the tree — kept alone it is the single shape `orphaned()` fires on.

**(grill pin, 2026-09-16 — the signature fate stays `erases`, by explicit deletion.)** An in-place rewrite would
let the blob survive as rubble, flipping `Collect`/`RemovePages` from `erases` to `breaks`. That silently reroutes
four gates: ADR-013's three `DocHash` anchors are all gated on "no signature" and would go QUIET rather than fire;
`p2p.ContributionProgress` hard-refuses `sign.Invalid` (`internal/p2p/l3.go:287-291`), making a reordered ceremony
document permanently un-contributable; `sign.SignApproval` refuses a document whose AcroForm still carries
`/DocMDP`; and `DropUAIdentificationUnlessSigned` reads "signed" at three sites. It would also falsify the
sentence the client already shows the user (`web/app.js:2845`). Whether an edited signed document should read
`invalid` or `unsigned` was already SETTLED — `/pending 455`, closed v1.128.98, decided as option B:
the refusal OBSERVES the bytes rather than predicting from the route, and option C (*"make every
route produce `invalid`"*) was refused at the line because redaction assembles a new document. So
keeping `erases` is continuity with a decision taken, not a fresh judgment.

Tasks:
- T01 — `pageselect.go`: one walk of the source page tree accumulating inherited `{Resources, MediaBox, CropBox,
  Rotate}`, producing the ordered leaf list. Not `ctx.PageDict` per page (`/pending 488`: that is O(pages²) — the item that measured and fixed it; this line credited `/pending 476` by mistake).
- T02 — selection through `api.PagesForPageCollection`; reject `≤0` and out-of-range before use; report in nib's
  voice, never pdfcpu's `pdfcpu: no page selected`.
- T03 — materialize all four inherited attributes onto each kept page, TOTALLY (an explicit `/Rotate 0` where a
  new ancestor would otherwise shadow), and emit a fresh flat `/Pages`: `/Kids` in selection order, `/Count`,
  `/Parent` repointed, `ctx.PageCount` set.
- T04 — a repeated page is deep-cloned (page dict, `/Annots`, each annot with a fresh `/P`). Mandatory:
  `ErrPageTreeDuplicate` (`model/recursion.go:80`, on the write path as well as validation) refuses a page
  object seen twice.
- T05 — the catalog/trailer allowlist: keep `{Type, Pages}` + `/Lang` + pruned `/AcroForm` + `/Names` reduced to a
  pruned `/Dests` (through `Node.Remove`, since `BindNameTrees` rebinds at write time). Everything else dropped.
- T06 — delete the AcroForm signature fields and `/SigFlags`, so a page operation still erases a signature.
- T07 — `Collect`, `RemovePages` (complement, ascending) and `collectWithoutStructure` route through the one
  primitive; `carryLang`'s two calls in the subset doors go (`/Lang` survives in place; three parses become one).
- T08 — the fixtures the repo lacks: nested inheriting tree, named destination, a form widget on a droppable page,
  an attachment + dropped-page canary, page-object identity, order identity.

**(built, 2026-09-16 — what the slice's own code review added to the T-list, recorded because the plan must
describe what was built.)** Three divergences and five additions, all from the review rather than from the
author:

- **T03's explicit `/Rotate 0` was NOT written, and does not need to be.** It was there to stop a new ancestor
  shadowing a page with no rotation of its own — but the flat `/Pages` this slice emits carries none of the four
  attributes, so there is nothing to shadow. Writing a `0` would add a key no document had.
- **T05 grew a second half.** Reducing `ctx.Names` is not sufficient: `BindNameTrees` re-binds the cache onto the
  catalog and only ever UPDATES the key it knows about (`xreftable.go:1363`), never removing a sibling — so the
  catalog's own `/Names` dictionary is rebuilt too. Without it a surviving destination carried `/EmbeddedFiles`
  out with it and a redaction shipped the source's attachments, measured with the payload readable in the bytes.
- **T06 grew four.** The signature pass now runs over EVERY page and BEFORE anything is cloned (a clone otherwise
  copies a signature widget onto the duplicate); it scans page `/Annots` for a merged `/FT /Sig` never listed in
  `/Fields`; it drops `/XFA`, which carries a whole XML copy of the form dataset and can hold its own signature;
  and it prunes `/CO`. Its earlier shape also removed non-signature siblings under a shared parent, which is
  integrity loss rather than erasure.
- **Three things no T named**, each closing a way a DROPPED page stayed reachable — and pdfcpu writes by
  reachability, so reachable means its `/Contents` is in the output: a kept field's `/Kids` is rewritten (it
  still named widgets on dropped pages); an annotation on a kept page whose `/Dest` or `/A /D` names a dropped
  page is unlinked; and the trailer's permanent `/ID[0]` is cleared, which the rebuild used to mint fresh.
- **T08 delivered more fixtures than it listed**: a tagged document (nothing else here is tagged, so the
  allowlist's `/StructTreeRoot` entry was graded by nothing), a signature split from its widget, a field shared
  across two pages, and a dictionary-shaped destination.

Acceptance:
- Every assertion the current subset tests make still holds — parity is the bar, not improvement.
- The catalog+trailer key set after `Collect`/`RemovePages` equals today's implementation's, measured both ways,
  on a fixture carrying an outline, labels, an attachment, a destination, metadata, a tree and `/Info`.
- A nested page tree whose pages inherit `/Resources`, `/MediaBox`, `/CropBox` and `/Rotate` renders identically
  after a subset AND after an arbitrary reorder.
- A named destination pointing at a dropped page does not survive as a dangling reference.
- `DuplicatePage` emits two distinct page objects, not one object named twice, each with its own `/Annots`.
- The signature fate stays `erases` and the tag-fate rows stay `dropped`; a marker string from a dropped page is
  absent from the output BYTES, not merely unlinked.


#### P02.S04b — subset operations carry the tree *(done 2026-09-16, v1.129.142)*
Scope: with the selection in nib's hands, prune the source tree in place — elements by `/Pg`, ParentTree `/Nums`
filtered, a cloned page's subtree cloned under a fresh key — and carry `/MarkInfo`, `/Metadata`,
`/ViewerPreferences` (identification dropped, ADR-032); refuse a nested ParentTree. `Booklet`'s rows flip with it;
`CarryAttachments`' census rows measure `Collect(src)`. Refs: D5.

**(grill pin, 2026-09-16 — the naive carry measures `carried`, and that is why the prune is the slice.)** Keeping
the four catalog keys and pruning NOTHING was measured on the census document (8 pages, 364 elements, flat
`/ParentTree`): `Collect(src, ["1"])` comes out **`carried` — the census's best verdict — while being a lie.**
217 completeness defects (7 unowned `/ParentTree` rows, ~210 dead-`/Pg` elements), 91,481 → 103,753 bytes, and
**229,859 decoded stream bytes in a one-page output**, because the dropped pages' content streams are still
reachable through the surviving elements' `/Pg`. This is S04a's re-anchoring hazard arriving through the tree,
and `fate`/`orphaned()` cannot see it — only `structureCarriedCompletely` (S01) can. So the slice's reader is
the completeness predicate, never the fate table, and the fate table's `carried` is a necessary condition only.

**(grill pin, 2026-09-16 — a subset that feeds a COMPOSITION does not carry, and three operations therefore do
not change.)** Measured: (a) `api.MergeRaw` keeps only the FIRST document's catalog and `/ParentTree`, so a
tagged document merged in after a carrying subset keeps its own `/StructParents` values pointing into the first
document's rows — a WRONG reverse link, the shape `tagState.undescribed` already records, and one the `partial`
verdict cannot distinguish from an honestly undescribed page; (b) pdfcpu's `CutPage` drops the tree outright
(`tree=false`, `elements=0`) but LEAVES `/StructParents` on the tiles it emits, which is the same shape one step
over. So `splice` (behind `InsertPDF`, and behind `replacePage` → `SplitPage`/`SplitRegions`) and `normalizePage`
route through the NON-carrying door, and `InsertPDF`, `SplitPage`, `SplitRegions` keep today's `dropped`
verdicts. That is not a deferral of D5 but a refusal to pre-empt **P02.S05/S06/S07, all three marked blocked on
Dan, whose question this is.** `SplitByBookmarks` and `SplitBySpans` are pure subsets with nothing composed
after them, so they carry, correctly and for free.

**(grill pin, 2026-09-16 — the containment is FOUR call sites, not two, and one of them is pure waste.)**
The attack enumerated every `Collect`/`RemovePages` caller from the code rather than from the design, and
`SplitRegions` has a `Collect` of its own on the tagged SOURCE (`pdfops.go:900`) that no reading of `splice`
reaches. Measured on a 73-page tagged document: the carry takes its prologue from ~37 ms to ~172 ms (4.6×) and
`normalizePage`'s `CutPage` then destroys the tree anyway, so the verdict is byte-for-byte the same `dropped`
— 100% waste. `SplitPage`'s `Collect(tilesBuf, ["2-"])` (`pdfops.go:805`) is benign only while `CutPage` drops
the tree, and an unnamed site is a live carry the day that changes. So the non-carrying door takes **four**
callers, each named at its site: `splice`, `normalizePage`, `SplitRegions`' own selection, and `SplitPage`'s
tile selection.

**(built, 2026-09-16 — the splitters DO carry, and the grill's reason for exempting them was a wrong number.)**
The attack measured one 73-page tagged document into 73 files at **2.416 s → ≈7.9 s (3.3×)** and recommended
routing `SplitBySpans` and `SplitByBookmarks` through the non-carrying door; that pin was written, the two call
sites were changed, and `/pending 531` was filed to undo it later. **Measured with the real prune instead of the
attack's prototype: 3.844 s → 4.378 s, 1.14×, all 73 parts `carried`, the fallback never firing.** The 3.3×
priced a failure path the finished code does not take — the prototype kept every element, so `carryIsComplete`
returned false on all 73 parts and each paid a second full read-change-write. The splitters are pure subsets
with nothing composed after them, D5 says they carry, and at 14% they do. `/pending 531` closed the same day it
was filed; the lesson is `CLAUDE.md`'s *"a REMEDY IS A CLAIM, and it is the one nobody re-checks"*, and this is
the instance — the remedy rode in on the finding's credibility and was one stopwatch from being wrong.

The gate's own cost is real and stays filed as **`/pending 530`**: `carryIsComplete` runs
`checkStructConsistency`, `parentTreeOwners` and `formDrawCounts` as **three independent page sweeps**, each
calling `ctx.PageDict` per page, and `PageDict` walks the tree from the root every call with no cache
(`model/xreftable.go:2141-2169`) — the O(pages²) shape `pageselect.go:79-81` already records as a measured 6 s
of a 27 s digest.

**(built, 2026-09-16 — what the carry COSTS, measured against the REAL implementation.)** Tagged text-only
documents, a full reverse permutation (the worst case: nothing is pruned, so every element is carried), single
runs on this machine — ratios are the signal, absolutes vary with load:

| pages | source | plain `Collect` | carrying | bytes | the gate alone | complete? |
|---|---|---|---|---|---|---|
| 8 | 103,451 B | 6 ms / 96,882 B | 14 ms / 103,393 B | 1.07× | 5 ms | yes |
| 37 | 151,406 B | 18 ms / 120,481 B | 72 ms / 151,120 B | 1.25× | 31 ms | yes |
| 73 | 211,491 B | 41 ms / 149,909 B | 145 ms / 211,110 B | 1.41× | 61 ms | yes |
| 146 | 332,739 B | 77 ms / 209,414 B | 294 ms / 332,259 B | 1.59× | 145 ms | yes |

So a tagged reorder is **2.3–4.0× slower**, the gate is 40–50% of that, and the output is up to 1.59× larger —
which is what a true tree costs. A half-document prune at each size comes out `carried` too, so the honest
fallback never fires on a well-formed document. Accepted for a tagged document, on a route with no timeout
anywhere (`cmd/nib/main.go:160` sets none, and `web/app.js:14337` says the client deliberately has none). The
byte ratios match the attack's prototype exactly and its timings did not; making the gate one page sweep rather
than three is `/pending 530`.

Two consequences worth writing down rather than discovering: a full-document reorder's output is now **~1.0× its
input rather than ~0.7×** (measured 211,491 → 211,110 B, within 400 bytes either way), so **ADR-005's byte cap
becomes newly reachable on a rearrangement, after the work is paid for** — the refusal then reads *"close a large
one first"* on a page reorder; and every later undo entry is 1.07–1.59× bigger against ADR-003's global pool.

**(grill pin, 2026-09-16 — `types.Dict.Clone()` does NOT deep-copy a subtree, and D-5 rested on it.)**
`Dict.Clone` recurses through `v.Clone()` and `IndirectRef.Clone()` returns `ir2 := ir` — a copy of the
REFERENCE (`types/dict.go:43-52`, `types/types.go:564-567`). An element's `/K` holds indirect references to its
children, so cloning the top element yields a clone whose children **are the originals**, and then repointing
"the clone's kids' `/Pg`" mutates the source subtree — `/pending 503`'s defect arriving from the other side.
The subtree clone therefore recurses through indirect references and allocates one new object per element with
`IndRefForNewObject`, which is what `clonePage` already hand-rolls for annotations and says why
(`pageselect.go:266-268`). `StreamDict.Clone()` is the same trap one level over: `sd1 := sd` shares the `Raw`
and `Content` backing arrays (`types/streamdict.go:68-82`).

**(grill pin, 2026-09-16 — a carry that anchors NOTHING passes the completeness gate, so the refusal is its
own.)** A prune can legitimately empty the tree: keep only pages no element describes and `structureCarriedCompletely`
is vacuously clean (no elements to walk, no keys to own), while `orphaned()` answers false because the kept
page's stale `/StructParents` still counts as an anchor (`tagfate.go:132-140`). The carry therefore refuses
unless the pruned tree still anchors something (`tree.anchored() > 0`), and that refusal lives in the carry —
**not** in `structureCarriedCompletely`, which `NUp` shares (ADR-009). With it, `orphaned` is unreachable on a
carried output by construction (completeness condition 3 forces an owned key, so `pagesSP > 0`), which is
asserted rather than assumed so the success path costs one parse and not two.

**(grill pin, 2026-09-16 — the gate reads the WRITTEN BYTES, and the fallback is the honest loss.)** An
in-context check cannot see what the write and the next optimizing read do: `clonePage` shares content streams,
so a duplicated page whose content draws an MCID-bearing form XObject draws it twice, which is condition 4 and
is exactly S02's fusion lesson one operation over. So the carry writes, `carryIsComplete` re-reads, and an
incomplete carry re-runs the selection with the carry off — the same shape as `completeOrHonest`. A source with
no `/StructTreeRoot` skips all of it, so the untagged document and the redaction path pay nothing.

Tasks:
- T01 — `structcarry.go`: `carryStructure(ctx, root, kept)` — the prune, in the source context. Elements,
  MCRs and OBJRs whose effective `/Pg` left the page tree are REMOVED from their parent's `/K`, never left
  dangling; an element that loses every kid goes; an element whose own `/Pg` died but whose MCR/OBJR kids live
  loses the `/Pg` and keeps the kids. An empty root `/K` is no carry.
- T02 — `/ParentTree`: rows whose key no longer has an owner are dropped, and a surviving slot naming a removed
  element is emptied. ~~Ownership is `parentTreeOwners` (S01), shared rather than restated (ADR-009)~~
  **— it could not be, and the exemption is declared at the site AND asserted by a test.** Renumbering has to
  WRITE a claimant's key back and `parentTreeOwners` returns descriptions (`"page 1"`), so the write side is
  `eachParentTreeClaim`, a second walk of the same three places. A declaration alone does not stop two walks
  drifting, so `TestTheClaimantWalksAgreeOnWhoOwnsAKey` compares their answers on three documents: a drift in
  the write side's direction silently drops the carry to the honest loss, and the other direction ships a row
  nothing can reach. The nested-number-tree refusal IS shared — `parentTreeDict`'s, already the one door.
- T03 — a repeated page gets its own key and its own elements: `allocParentTreeKey` for each source key the
  clone's own objects claimed (`/StructParents`, and each cloned annotation's `/StructParent`), the row's
  elements deep-cloned once each, `/Pg` and an OBJR's `/Obj` repointed at the clone, each clone inserted into
  its original's parent `/K` immediately after the original, rows written through `setParentTreeSlot`.
- T04 — the four catalog keys are EARNED: `catalogAllowlist` keeps its default-deny shape and the carry re-adds
  `{StructTreeRoot, MarkInfo, Metadata, ViewerPreferences}` only on success. ADR-032 is already discharged —
  `rewriteContext` drops the identification before `fn` runs.
- T05 — the doors: one primitive `selectPages(ctx, keep, carry bool) (carried bool, err error)`, and the two
  named wrappers ~~`selectPagesCarrying` / `selectPages`~~ **at the BYTES level instead — `subsetCarrying` and
  `subset`** (built 2026-09-16), because the output gate the carry needs can only read the bytes it wrote and
  so has to live where the write completes. `Collect` and `RemovePages` take the carrying one;
  `collectWithoutStructure` takes the plain one, and its callers are `RedactPages`, `splice`, `normalizePage`,
  `SplitRegions`' own selection and `SplitPage`'s tile selection, each exemption named at its site (ADR-009).
  **The two splitters carry** — see the measurement above.
- T06 — the output gate, the anchors-nothing refusal, and the honest fallback (the pins above).
- T07 — the census flips: `Collect`, `RemovePages`, `DuplicatePage`, `Booklet` → `carried`, their `pageSetLoss`
  rows in `knownUA1Deltas` removed; `CarryAttachments` re-driven with `Collect(src)` as its destination, which
  is what the server's delete and reorder routes actually do (`internal/server/pages.go:77-81`).
- T08 — the two inherited debts: S03's `TestRedactionEmitsNoStructureTree` probed RED against a carrying
  `Collect`, and the S04a inventory's **G6** row — the AST routing guard reads `RedactPages`' callees only, so
  it says nothing about what `collectWithoutStructure` itself calls.
- T10 — the two server tests whose SETUP asserts that `Collect` drops the claim
  (`internal/server/tagnotice_test.go:89-96` and `:149-158`, on a ONE-page fixture, so the carry is the
  identity selection and both `t.Fatal`) repointed at an operation this slice keeps at `dropped`.
- T09 — the fixtures the repo lacks (measured absent: the census document has **no MCR, no OBJR, no RoleMap and
  no annotations at all**): an OBJR-referenced annotation on a kept AND on a dropped page, a nested
  `/ParentTree`, a RoleMap, an MCR kid naming another page, an annotation carrying `/StructParent`, and a page
  drawing an MCID-bearing form XObject for the duplicate case.

**(built, 2026-09-16 — three things the slice's own code review added to the record.)**

- **The subset PRESERVES its input's fate rather than setting it, and the census's single declared verdict
  cannot say that.** `Collect` declares `carried`; measured on `Append(taggedFixture, untagged)` — the
  `api.MergeRaw` shape every ceremony document takes — a reorder of that document comes out **`partial`**, and
  keeping only the undescribed page comes out `dropped`. Before this slice `dropped` was unconditional. **A
  subset does not MAKE a partial document**: the appended page was already undescribed under `/Marked true`,
  and what the old behaviour did was launder that by destroying the whole live tree — which ADR-031 records as
  the cure being worse than the disease at the only scale that matters. Preserving it is continuity with a
  decision taken. `TestASubsetPRESERVESItsInputsFateRatherThanSettingIt` is the reader, and it is stronger than
  the census row because it grades four inputs rather than one.
- **`carryIsComplete` is SHARED with `NUp`, and it gained a condition.** The orphan-page condition — no
  `/Type /Page` object outside the page tree — is a property of the document rather than of the tree, so it
  went into the shared gate deliberately while the anchors-nothing refusal stayed in the carry. `NUp` was
  measured before and after: still `carried`, veraPDF differential still green. No T named it; recorded here.
- **`/Metadata` and `/ViewerPreferences` are conditional on the source carrying a `dc:title`.** T04 says the
  carry re-adds four keys; it re-adds two unconditionally and two only where there is a title to state, because
  `/DisplayDocTitle true` over no title is the one state `SetTitle`'s own door refuses (*"a viewer told to
  display a title it cannot find shows an empty chrome bar"*). A titleless tagged document's carried subset
  emits `{Lang, MarkInfo, Pages, StructTreeRoot, Type}`.

**(built, 2026-09-16 — what the slice's own code review CHANGED, recorded because the plan must describe
what was built.)** Four reviewers over the diff, three of them measuring; nine defects in code I had written
and reasoned about, every one measured rather than argued:

- **A removed element was RESURRECTED by the clone path, `/AF` and all.** `carryOntoClone` read a
  `/ParentTree` row before the removed elements were cleared out of it, so `DuplicatePage` deep-copied an
  element the prune had taken out — one that kept the `/AF` and `/T` the prune strips only from survivors —
  and attached the copy to the tree. Measured: `Collect(src, ["1","1"])` shipped the source's embedded payload
  and its filename with `fate=carried`, **0** completeness defects and **0** orphan pages, so the gate
  certified it, and `pruneNames` had already deleted `/EmbeddedFiles` so `Attachments()` reported none. Fixed
  by ORDER: `clearRemovedFromRows` now runs before any clone.
- **An element's own `/Pg` decided its fate ONCE, across different parents.** Memoizing on the object alone
  cached a judgment taken under a dead page. Measured: the same fixture came out `carried` keeping page 1 and
  **`dropped`** keeping page 2. The memo is keyed on the object AND the inherited page.
- **A copied child's `/P` named the ORIGINAL's parent** — a tree that disagrees with itself in the two
  directions a reader walks it, with no reader in this package to see it (`internal/uacheck`'s
  `declaresLangFor` climbs `/P`). `kidsOf` now takes the copy's ref and writes it.
- **The OBJR rule contradicted its own comment.** Requiring a live page unconditionally destroyed a grouping
  `/Form` element with no `/Pg` — the one shape PDF/UA asks for, and what `anchored()`'s kid-walk was added for
  at P06.S07. A page is now checked only where one is NAMED.
- **`eachParentTreeClaim` was wrong three ways**: a form XObject in two pages' resources was offered twice, the
  second time with the key the first offer had just written (`[0 4 1 101]`, four offers for three objects); the
  enumeration was Go map order, so a carried subset's `/Nums` numbering differed run to run (`[0 2 3]` on 15 of
  24 reads, `[0 3 2]` on 9); and an annotation's `/AP` appearance stream claims a key and was reached by
  neither walk — measured, its row was dropped while the stream went on naming it, with the output reported
  clean. One shared visited set, sorted names, and `/AP` walked. `parentTreeOwners` got the shared set too.
- **A corrupt source row failed the whole operation.** `of()` returned an error on a slot naming an absent
  object, so `DuplicatePage` refused the document outright — 0 bytes — where the same document's `Collect`
  succeeded. A defect in the SOURCE produces the honest loss; the slot is skipped.
- **A refusal reached after renumbering left `/StructParents` rewritten** on objects that survive the
  allowlist, against this file's "a refusal costs nothing and needs no rollback". Renumbering is now last,
  after the final refusal.
- **`carryTitleFloor` could fail the operation and broke `SetTitle`'s one-door rule.** It had four error
  returns and ran after the structure keys were restored; and it wrote the XMP packet and the preference while
  skipping `/Info`'s `/Title` — exactly the two-of-three state that door's header exists to prevent. It now
  returns a bool (a failure drops the carry), and writes through `setInfoTitle`.
- **`/DisplayDocTitle` was INVENTED, not carried.** Measured: a source saying `false` and a source saying
  nothing both came out `true`. A subset states what the document stated.
- **`orphanPageObjects` was O(pages²)** via `PageDictIndRef` per page — 238 ms at 800 pages against **175 µs**
  for its whole xref sweep. It takes the caller's live set now.

**And five instruments could not fail for the reasons they named**, each proved by mutation:
`TestACarriedOutputIsNeverOrphaned` passed over an empty population (all three cases skipped) while being
cited in production as the reason the gate reads the document once; the nested-`/ParentTree` test's control
read the fixture's own bytes rather than a `Collect` of them; `TestACarryThatAnchorsNothingIsRefused` passed on
a build that refused everything; the "AND a reorder" clause had no reader; and **a carry that deleted
`/RoleMap` from every subset left the entire suite green**, veraPDF included — an element typed `/Para` means
nothing without the map that says it is a `/P`. `carryOf` also reported "clean" for a tree it could not parse.
All six now have readers, plus a stimulus for the orphan-page condition, which had never been seen to decide
the gate.

Acceptance:
- `Collect`, `RemovePages`, `DuplicatePage`, `Booklet` census rows add nothing; tag-fate verdicts `carried`.
- A fixture with an OBJR-referenced annotation on a kept and on a dropped page.
- S03's `TestRedactionEmitsNoStructureTree` is probed RED against a carrying `Collect` — the debt S03 recorded.
- **(added at the grill)** `structureCarriedCompletely` is EMPTY for every carried output, and a dropped page's
  marker string is absent from the output's decoded streams — the fate table cannot see either.

#### P02.S05 — crop carries the tree *(done 2026-09-21, v1.138.11)*
Scope: `Crop` wraps pages in place instead of rebuilding them. Measured compliant even at a 5% window; the
question is what a reader should hear. Refs: D5.

**(decision, 2026-09-21, via /discuss, amended by /grill)** The tree is carried. **A structure tree describes the
FILE, not the view**: a crop clips, it does not remove — the text still extracts, copies and searches — so the
tags keep describing it, and dropping them would lose every visible element to avoid a mismatch on the hidden
ones. The ADR written with this slice states that principle and its boundary (S06 is the case it refuses).
~~Crop UI says hide-not-remove~~ — **already true**, `web/index.html:1626` says cropping *hides* and points at
Flatten or Redact; nothing to build. **Follow-up, not this slice:** turning elements wholly outside the window into
artifacts. Cheaper than the round believed — the per-element extent already exists (`structview.go:62,183`) and so
does the artifact edit (`structartifact.go:41`); the unbuilt part is applying one to the other.

**(grill, 2026-09-21 — the scope line was the wrong shape: crop moves the BOX, it does not wrap the content)**
The sketch said *"wraps pages in place instead of rebuilding them"*. Moving the page's `/MediaBox` in its
own coordinates does strictly less and carries strictly more: the content stream, `/Rotate`, MCIDs,
`/StructParents` and every annotation `/Rect` stay in a space the crop never moves, so there is nothing to
remap and no carry to gate. A content wrap would have translated the page to the origin and so had to
translate every annotation too — and the rebuild it replaces DELETED them (`CutPage` drops `/Annots`), which
the grill found was a second, unreported loss on every cropped page. ADR-047.
- T01 — `Crop` rewrites target pages' `/MediaBox` in place through `rewriteWithConf` (`cropWindow` maps the
  display-space fractions per `/Rotate`, over CropBox ∩ MediaBox); an inherited CropBox is overridden.
- T02 — `tagFates` `Crop` → `carried`; `knownUA1Deltas` loses its `Crop` row; the generated digest golden's
  `cropped` row moves (input changed; every other row identical).
- T03 — readers: `TestACropCarriesTheStructureOfEveryPage`, `TestCropMapsTheDisplayWindowAtEveryRotation`,
  `TestCropReadsTheBoxAndRotationAPageINHERITS`.
- T04 — the server's lost-tagging notice tests move their stimulus from `Crop` to redaction, whose drop is a
  decision rather than a slice.
- T05 — ADR-047; README's accessibility and Crop passages.
- T06 — *(added at the build)* `test/ui/crop.test.mjs`, the real mouse-drawn box through the real binary; tier 3's
  file pin 38 → 39.

Acceptance: a tagged multi-page document cropped comes out `carried` with no completeness defect, every
element and every annotation kept; each of the four rotations maps the display window the user drew; the
veraPDF differential adds nothing for `Crop`.

#### P02.S06 — split pages *(done 2026-09-21, v1.138.14 — code in S07b, decision ADR-049)*
Scope: `SplitPage`, `SplitRegions`. Tiles are clones of the page dict carrying the whole content. Refs: D5.

**(decision, 2026-09-21, via /discuss, amended by /grill)** **The tiles carry no subtree.** A per-tile clone makes
the DOCUMENT read each split page N times — a 2-up spread reads both halves on each half — which is a false
account of structure, not a view mismatch; that is where S05's "describes the file" principle stops, and the ADR
says so. **But the rest of the document keeps its tree.** Both operations reach the merge through `splice`
(`replacePage`, `pdfops.go:934,983,1061`), so today one split page destroys every page's tags; once S07 lets
`splice` carry, the tiles land undescribed under the document's existing claim and the verdict moves
**`dropped` → `partial`** — ADR-031's own reasoning (the cure must not be worse than the loss). Hence S06 builds
after S07. **Follow-up, not this slice:** a positional partition (each tile keeps the elements inside it and
artifacts the rest) — rects and the artifact edit exist, as in S05's follow-up; the unbuilt part is rewriting
each tile's content.

**(close, 2026-09-21)** Nothing was left to build: S07b routed `SplitPage`/`SplitRegions` through the
host-preserving `splice`, so the census rows moved (`knownUA1Deltas`: `pageSetLoss` → `7.1 t3`, `7.4.2 t1`) and
`TestSplittingAPageKeepsTheRestOfTheDocumentsTree` reads both halves of this decision — the tiles carry no
claim, every other page keeps its own. What S06 owed was the decision record: ADR-049.

#### P02.S07 — merges graft the second tree *(done 2026-09-21, v1.138.13 — S07a v1.138.12, S07b v1.138.13)*
Scope: a context-level graft (`pdfcpu.MergeXRefTables`) offsetting the second document's keys and merging root
`/K` and RoleMaps; ~~`InsertPDF` inherits it with S04~~ **— it does not: S04b routes `splice` through the non-carrying door and defers `InsertPDF` here in full** (struck 2026-09-16). Refs: D5, ADR-031.

**(decision, 2026-09-21, via /discuss, amended by /grill)**
- **Graft for `Append`, `Combine` and `splice`** (so `InsertPDF`, and S06's operations, leave the non-carrying
  door). Tagged + tagged comes out `carried`.
- **A graft only EXTENDS a claim the first document already makes (added at the grill).** Where the first input
  carries no live tree, the second's tree is not carried and its `/StructParents` are stripped — the result is
  untagged, exactly as today. Without this, S09's tagged readme would graft into every untagged co-signed
  contract and make it claim tagging over pages nobody tagged: ADR-031's worst case, a screen reader abandoning
  its fallbacks. It also keeps ADR-031's recorded rule that argument order decides the fate.
- **Mixed inputs stay `partial`.** This AMENDS ADR-031's note (a new ADR citing it); it does not supersede the
  verdict, which stays true for tagged + untagged.
- **An incomplete graft falls back to stripping the second document's `/StructParents`**, so no page keeps a
  reverse link into the other document's rows (grill pin (a), 2026-09-16, above) — through ONE door, as `honest`.
- **Per-slice deepdive owed** on the `MergeXRefTables` graft before the grill; it was not dived here.

Acceptance: untagged-first + tagged-second, and tagged-first + untagged-second, each asserted in both argument
orders — the former claims nothing; a tagged + tagged merge is `carried` with veraPDF adding nothing over its
inputs; no merged page's `/StructParents` resolves into the other document's rows.

**(pre-slice deepdive + grill, 2026-09-21 — two premises of the decision above were false against the code, and the
slice splits)**
- **The defect is LIVE in `Append` and `Combine`, not only in `splice`.** Probed: `Append(tagged, tagged)` writes
  both pages with `/StructParents 0`, so page 2 resolves to document 1's element — fate `partial`, one completeness
  defect, `carryIsComplete` false. The census cannot see it: it drives the merges with an UNTAGGED second document
  only (`tagtable_test.go`). So "exactly as today" in the extend-only bullet was wrong; S07a is a bug fix too.
- **"The first document" is the wrong host for `splice`.** Inserting before page 1 makes the INSERTED document the
  first input, so a per-`Append` extend-only rule would strip the original's whole tree when an untagged cover page
  is inserted at page 1 and keep it at page 2. The rule is: **a merge only extends a claim the HOST makes**, and the
  host is the first document for `Append`/`Combine` (the user chose that order) and the ORIGINAL document for
  `splice` at every insertion point. That is the decision's own principle stated precisely, not a new decision.
- **The graft cannot sit behind `api.MergeRaw`.** `MergeXRefTables` renumbers the second document's objects in
  place and frees its catalog, so its tree is unreachable in the written bytes; the hook is between the merge and
  the optimize, and nib owns the ~15-line loop (`MergeRaw`'s config, the PDF 2.0 refusal, and a page-count check
  for the error `merge.go:1098` swallows). The source's claims are offset IN THE SOURCE CONTEXT before the merge
  (`eachParentTreeClaim` + its `/Nums`), and its root is attached afterwards through the catalog map the merge
  patched in place.
- **Splits:** S07a — `Append`/`Combine` through one merge door; S07b — `splice` with the original as host, which
  also has to place the inserted elements in reading order rather than at the root's end.

S07a tasks:
- T01 — `mergeDocs` in `internal/pdfops/merge.go`: nib's own merge loop; host = first input; graft a source whose
  tree is flat, readable and whose RoleMap/ClassMap do not conflict with the host's; otherwise strip its claims.
  Root kids appended with `/P` repointed; a differing source `/Lang` stamped on each grafted top-level element;
  `/ParentTreeNextKey` advanced; source `/IDTree` dropped; tier lowered (or unrecorded if either is). UA claim
  dropped in-context (ADR-032). Output gated on `carryIsComplete`; an incomplete graft re-merges strip-only.
- T02 — `Append` and `Combine` call it; `withoutUAClaim`'s second write goes.
- T03 — census: tagged+tagged drives for `Append` and `Combine` (`carried`), untagged+tagged (claims nothing);
  readers for offset keys, RoleMap merge, conflict fallback, `/Lang` stamp, strip of stale claims.
- T04 — ADR-048 amending ADR-031's merge note.
- T05 — *(added at the build)* the bogus-key detector's stimulus rebuilt by hand, since the merge no longer
  makes one; a root `/K` holding one DIRECT element is read (the slice's code review); `/pending 559`
  amended — the union it asks for now has a place to stand at the graft's hook.

S07b tasks *(done 2026-09-21, v1.138.13)*:
- T01 — `splice` merges the WHOLE original with the inserted document in one `mergeOnce` (a `finish` hook), then
  sets the page order there with the carrying `selectPages`; the original is the host at every insertion point.
  Its old shape is `spliceWithoutStructure`, the fallback when the output gate refuses.
- T02 — `placeInserted`: the grafted elements move from the root's end to before the first host element that
  starts after the insertion page, at the first level holding more than one element (descending through any
  single-element wrapper, stopping at content). `firstPage` locates an element by its lowest MCID/MCR/OBJR page.
- T03 — census: `InsertPDF` `dropped` → `partial`; the veraPDF deltas for `InsertPDF`, `SplitPage` and
  `SplitRegions` shrink to what each genuinely does (7.1 t3 for untagged inserted pages or tiles, plus
  7.4.2 t1 where the drive removes the page carrying the only /H1); the routing guard reclassifies `splice`.
- T04 — readers: placement in reading order (with a link-first, an unlocated-element and a `/Part`-wrapped
  variant), before page 1 (the host stays the original), before the last page, a split keeping the rest of the
  document's tree, never inside an element's content, and `firstPage`'s two rules pinned directly.
- T05 — README's accessibility passage: combining and splitting keep tags; only redaction still loses them.

#### P02.S09 — nib's own pages are tagged *(done 2026-09-22, v1.138.15)*
Scope: the trust-explainer readme (`p2p/readme.go:306`, through `PrepareDocument` — so **every co-signed
document**, `server/cosign.go:455`, not only ceremonies), the ceremony page and the signature pages
(`sigpages.go:229,238`) are rendered with a structure tree, as **real content** — never artifacts, which would hide
the trust explanation from assistive technology on purpose. Tagged inside `PrepareDocument` /
`PrepareCeremonyDocument`, before any signature. Prefer `mdpdf`'s tagged output (ADR-033) over a second tagger if
the readme's placement — `readmeFloor`, the block-stack offset — survives it; measure, don't assume. Refs: D5.

**Declared gap:** signature widgets added at signing stay untagged (7.18.1). **Not impossible — unattempted:**
tagging one rewrites `/StructTreeRoot` and `/ParentTree` inside the signing revision, and whether earlier
signatures' validators report that as a disallowed change is unmeasured (filed to `/pending`).

Acceptance: a tagged document through `PrepareCeremonyDocument` comes out `carried` — S07's fallback strip
firing here is RED, not a silent `partial`; an untagged one comes out claiming nothing (S07's extend-only rule);
veraPDF adds no failure on the appended pages.

**(pre-slice deepdive + grill, 2026-09-22)** The render stays pdfcpu's create-from-JSON — through `mdpdf` it would
relayout and `readmeFloor`/`ErrReadmeOverflow` would stop describing it. `tagOnePage` already tags a page from one
role per text run, and nib knows every line's role when it draws it, so the tags are EXACT: the autotagger would
record `Inferred` and the graft's `lowerTier` would downgrade an `Exact` host. Two traps named by the dive and
pinned by readers: tag AFTER the language declaration (`declareContentLang` skips marked pages), and record the
tier (a fragment with none deletes the host's). Measured before: a tagged host left `PrepareCeremonyDocument`
with 35 unmarked text runs at 2 signers, 36 at 8. ADR-050.
- T01 — `pdfops.TagAuthoredPages`, sharing `tagMarkdown`'s tail (`tagFromRoles`); `unmarkedTextRuns` exported
  as `UnmarkedTextRuns` (it had a production caller already, so no exemption).
- T02 — `RenderReadme`, `renderCeremonyPage`, `renderSignaturePage` build roles beside their lines and tag
  after the declaration; `TestTheDeclarationClaimsNoTagging` inverted into `TestTheReadmeIsTaggedAndStillDeclaresItsLanguage`.
- T03 — readers: wholly tagged through co-sign and ceremony at 2 and 8 signers (tier Exact kept), an untagged
  host not made to claim, veraPDF adds nothing, each page's elements, the orphan refusal.
- T04 — ADR-050; README; the langdoor rationale, tagwrite's door list and the readme's "unpayable" comment
  corrected (nothing re-renders these pages — searched).

#### P02.S08 — MCIDs inside a Form XObject are reached through MCR dictionaries *(done 2026-09-16, v1.129.144)*
Scope: replace element-level `/Stm` (`tagcarry.go:316`) with MCR dictionaries carrying `/Pg` and `/Stm`, after
reading ISO 32000-1 tables 323–324 and one screen-reader run. Refs: D5.

**(pin, 2026-09-16, pre-slice deepdive — the slice gains the READER half, and the line number was wrong)**
`~/.claude/projects/-home-dan-repos-nib/memory/deepdives/2026-09-16-the-n-up-carry-and-marked-content-references.md`
(the project memory directory, as with the seam inventory named under *Standing caveats* — not a repo path). Three things the sketch could not
see:
- **Table 323 defines no `/Stm`**, so today's key is not merely redundant — a conforming reader ignores it
  and resolves the MCID against `/Pg`'s own stream, which is exactly the failure the comment at
  `tagcarry.go:255-258` claims it prevents. **Table 325 defines none either**, so the `OBJR` arm of
  `tagcarry.go:301` is the same defect.
- **The acceptance as written cannot fail.** Measured on a 61-element fixture: `nib ua` and veraPDF report
  the *identical* clause set for the source and its `NUp(2)` (`5 t1`, `7.2 t33`, `7.2 t34`, all from the
  fixture's missing `/Lang`). Both are blind to this by construction — `uacheck` discards every
  marked-content kid at `structure.go:201-202`, and `7.2 t34` resolves content→element through
  `/StructParents`, the direction that has always worked. So a writer-only slice ships with no instrument
  that moves.
- **What DOES move, measured: 31 of 61 elements read the wrong text after the carry.** `structview.go:142`
  keys text by `(page, mcid)` and `textrun.go:624` flattens a form's MCIDs under the page key, so after an
  n-up an MCID in one form is indistinguishable from the same MCID in the other and both texts are
  concatenated onto both elements — element 27 reads `"TitleParagraph number 30…"` where its source read
  `"Title"`. Teaching the reader to honour `/Stm` is therefore **in scope**: it is what makes the writer
  half observable, and without it `nib tag tree` stays wrong on nib's own output. Additive — the
  `(page, mcid)` index is untouched and only a kid carrying `/Stm` consults the narrow one, so no document
  without `/Stm` changes. **Reader first, then writer**: the reader half alone is inert.

Acceptance:
- The spec reading is quoted; veraPDF and uacheck still pass the carried `NUp`.
- **(pin)** No `StructElem` or `OBJR` in a carried output carries `/Stm`, and every marked-content kid whose
  content lives in a form is an MCR naming that form.
- **(pin)** A carried `NUp`'s elements read back the text their source elements read — the content-level
  assertion no n-up test makes today.
- **(pin)** One screen-reader run over the carried output, recorded with what was heard.

**(close-out, 2026-09-16, v1.129.144 — ADR-038.)** All four clauses met, and the first one could not have
failed: `nib ua` and veraPDF report the **identical** ua1 clause set for the source, the broken carry and the
repaired carry, so the original acceptance was a non-regression check over an instrument blind to the change.
What graded the slice is the pin's clauses.
- **Text.** 8-page, 241-element population: `nup --n 2` had **118 of 241** elements reading two source pages'
  words concatenated, `--n 4` had **180 of 241**. Both are **0** after. The 61-element population went 31 → 0.
- **Geometry**, unasked-for and user-visible: element 27's `rect` went from `[104.0, 44.8, 360.9, 453.6]` —
  a box unioned across two source pages — to the source's own box under the n-up's scale. That rectangle is
  what the Tags panel highlights.
- **Cost, measured and not projected.** `−0.33%` to `+0.05%` across the range. Arithmetic had said `+2.8%`;
  at `n=4` the output is **smaller**, because 241 elements each shed a `/Stm` key and near-identical MCR
  dicts compress to almost nothing in an object stream.
- **The screen-reader run is a FINDING, not a confirmation.** Evince over AT-SPI — what Orca speaks — returns
  byte-identical text for the broken and repaired carries, in reverse paragraph order: poppler extracts
  geometrically and never opens the structure tree, so the whole Linux stack cannot observe this in either
  direction. The consumer that does honour the rule is **pdf.js**, nib's own viewer and Firefox's, which
  reads `/Stm` on an MCR and has no branch that would read it off an element.
- **The slice's own review found three criticals in it**, all in code written the same day: a `/K` spelled as
  a single dictionary — ISO 32000-1's own Example 2 — dropped the entire tag tree; a run was stamped with the
  stream its glyphs were drawn in rather than the one its sequence was opened in; and an element inheriting
  `/Pg` kept bare integer kids. A fourth, a vacuous test that executed zero assertions, was proven by running
  it. Residue: `/pending 536`, `/pending 537`.

### P03 — Checker: structure-tree containment and roles (~40 rules) *(done 2026-09-22, v1.144.1)*
**Goal.** Tables, lists, TOC, headings, notes, Form/Link elements and role maps, each agreeing with veraPDF on
its corpus files. Refs: law 1.

**Exit criteria.** Every rule in the family passes `veracorpus_test.go`; rules without corpus files agree with
veraPDF on fixtures of their own.

**(phase-open, 2026-09-16, v1.129.144)** The family is **43 rules, not ~40**, and nib implements **three** of
them (`7.1 t11`, `7.4.2 t1`, `7.5 t1`), so P03 lands **40**. Counted from veraPDF 1.30.2's own profile —
`org/verapdf/pdfa/validation/PDFUA-1.xml` inside `~/verapdf/bin/cli-1.30.2.jar`, 106 rules total, which
confirms the figure this plan has carried since P01.

- **Two thirds of the family is ONE shape.** veraPDF writes most of it as two predicates over resolved role
  names — `parentStandardType == 'X'` and a regex over `kidsStandardTypes`. Tables, lists and TOC differ only
  in which names fill them. That is a containment **matrix** with one door, not twenty-five hand-written
  rules (ADR-009), and S02 builds it.
- **THE PHASE'S TRAP, and it is specific to this corpus.** For **twenty** *(pin, P03 phase close: the list names
  **twenty-three**, and all twenty-three were checked to hold a pass fixture containing their subject)* of the family's clauses the corpus
  holds **only fail fixtures** — `7.2 t4-t14`, `t18-t20`, `t36-t38`, `t41-t43`, plus `7.1 t11`, `7.4.4 t3`,
  `7.5 t2`. A rule that returns `Fail` unconditionally scores **perfectly** on every one of them, and
  `veracorpus_test.go` cannot tell it from a correct rule: it scores false-pass and false-fail, and an
  always-Fail rule produces neither. **Every clause in that list owes a pass fixture of its own**, and the
  slice that lands it says so. This is checklist item #19 at phase scale — the judgment is graded and the
  stimulus never arrives.
- **Eleven of the 106 have no corpus file at all**; five are ours — `7.1 t12`, `7.2 t16`, `t28`, `t39`,
  `t40`, `7.18.4 t2`. Those are the exit criterion's second half, and they need a fixture *and* a veraPDF run
  to say what it answers, never a reading of the rule text.
- **`7.18.7 t1` has five corpus files and no rule in the profile**, so those files are never exercised by any
  ua1 validation. Not ours to fix; recorded so nobody counts them as coverage.
- **`7.2 t42` and `t43` are the same sentence with opposite polarity** on `wrongColumnSpan`. They are one
  implementation and must land together or the pair is incoherent.
- **Three places encode the rule count and must move with every batch**: the floor at `uacheck_test.go:132`,
  the `corpusReach` table at `veracorpus_test.go:45-51` (a row per clause, the number **measured** over the
  297-file set — `veracorpus_test.go:187` fails a clause with no row), and prose at `uacheck.go:112`,
  `door.go:22`, `rules_catalog.go:334`.

**Phase close (2026-09-22, v1.144.1).** Acceptance ledger over the exit criteria and every phase-open amendment:
- *"Every rule in the family passes `veracorpus_test.go`"* — **met**: 297 files, 17,072 scored pairs, 0 false pass,
  0 false fail; 42 of the family's 43 clauses registered, each with an exact `corpusReach` row. The 43rd, `7.1 t12`,
  is **declared, not registered** (S06's pin: veraPDF 1.30.2 cannot fail it for an element reached through the tree).
- *"rules without corpus files agree with veraPDF on fixtures of their own"* — **met**: `7.2 t16/t28/t39/t40` (S03,
  eighteen measured trees), `7.18.4 t2` (S06, six documents), `7.1 t12` (five fixtures, five recorded passes); the
  in-repo oracle runs veraPDF live (`TestTheOracleValidatesTheChecker`, 2,610 of 2,610 pairs).
- *"The family is 43 rules … P03 lands 40"* — **met as 39 registered + 1 declared** (19 → 58 in the registry).
- *"Every clause in that list owes a pass fixture"* — **met**: all 23 listed (the note said twenty) hold a unit pass
  fixture containing their subject, checked by the phase review; the always-Fail probes are red-proven (S02, S03).
- *"Those [no-corpus clauses] need a fixture and a veraPDF run"* — **met** (as the second criterion).
- *"`7.2 t42` and `t43` … must land together"* — **met** (S04).
- *"Three places encode the rule count and must move with every batch"* — **met**: 58 at every site; the README and
  parity figures are now asserted from `len(Clauses())`, not a literal.
- Law 1 / law 4 under hostile input — **not met at the review, met at close**: the phase review found the containment
  walk exponential on shared pass-through elements (P03.S02's code) and the content walk unbounded on form fan-out;
  both now answer CannotCheck naming why, with the table grid and a `/P` loop bounded the same way. Corpus verdicts
  are byte-identical before and after.
Required-run gates: tiers 4 and 6 **did not fire** — P03 and its fix pass touch `internal/uacheck`, `internal/pdfops`
(the role resolver), tests and docs, none of `internal/server`'s session/ceremony/delivery/discovery paths,
`internal/p2p` or `internal/rendezvous`. Tiers 0–3 ran over the committed tree (the phase's `/tidy`). Review:
`code-reviews/v1.144.0-p03-phase-close-2026-09-22.md`; out-of-scope residue `/pending 613-634`, three of them critical
(613 signer fingerprint, 614 pdfcpu colour-space crash, 615 displaced-arm leak). Graduation: 36 rows, 34 keep-live
mechanically, 2 declarations kept, 0 hot-path; seven bound rows added.

#### P03.S01 — role resolution is the spec's algorithm, and the checker owns the standard set *(done 2026-09-22, v1.139.0)*
Scope: `standardType` (`uacheck/structure.go:91`) implements the wrong algorithm, not merely a tight bound.
It counts to ten hops and breaks only on a self-map, then **returns the intermediate name** — a silent false
Pass under law 4, which three registered rules already consume (`rules_headings.go:40`,
`rules_semantic.go:63`, `:114`). ISO 32000-1 §14.7.3 Note 2: circular chains *"are explicitly permitted …
A conforming reader using the role map should follow the chain of associations until it either finds a
structure type it recognizes or returns to one it has already encountered."* So: a visited set, and a
recognition test against §14.8.4's set, which the checker must hold itself — `pdfops.standardStructTypes`
(`structedit.go:65-74`) exists but `uacheck/structure.go:12-22` gives the standing reason the checker does
not borrow the writer's model, and that reason holds. Lands `7.1 t5`, `t6`, `t7`. Refs: law 1, law 4.

**Note the shape of the defect, because it is the argument for this slice going first.** `pdfops` has a
second resolver (`structview.go:277-286`) and it is wrong in the **same** way. The independence between the
two models is deliberate and protects against one implementation's bugs; it cannot protect against a shared
misreading of the spec, which is what this is. Absorbs `/pending 507`'s first clause.
Acceptance:
- A role map with a two-name cycle resolves to a recognised type or is refused — never to the intermediate.
- A chain longer than ten hops that terminates at a standard type resolves; today it does not.
- `7.1 t5`, `t6` and `t7` agree with veraPDF on their corpus files, each with a `corpusReach` row measured.
- A pass fixture for each of the three, because `7.1 t5-t7`'s corpus coverage is not all-pass.

**Two thirds of this slice is already spent.** `/pending 507` landed the resolver and `/pending 548`
landed **`7.1 t6`** — registered, `corpusReach` 294, pass and fail fixtures in both the unit set and the
oracle corpus, six red proofs. What is left here is `7.1 t5` and `t7`, and two things 548 measured that
neither of them may be built without:
- **The corpus's file names are the specification's test numbering, not veraPDF's rule numbering.**
  `7.1-t05-fail-d.pdf` is a 7.1-**6** failure and veraPDF PASSES 7.1-5 on it. Read the clause off a
  veraPDF run, never off a file name; a rule registered under the wrong number is compared against the
  wrong oracle on all 297 files and the corpus guard agrees with it.
- **These clauses are element-scoped.** veraPDF's object is `PDStructElem` for `7.1-6` and `7.1-7` and
  `SENonStandard` for `7.1-5`, so the verdict is per element and a fact about the dictionary alone is not
  a failure.

**(grill, 2026-09-22)** Measured on veraPDF 1.30.2 before a line was written — the whole corpus (297 files)
plus eight `roleMapDoc` fixtures of nib's own — and it re-cut three premises:
- **The typing walk and the cycle walk are two questions.** `/Alpha → /H1` with `/H1 → /H1` FAILS 7.1-6 on
  `/Alpha` and types it `/H1`. `/pending 548` derived circularity from the typing walk; once the typing walk
  stops at a recognised type it cannot, so each gets its own walk.
- **Recognition stops a chain only after a mapping step.** `/TR → /TD` types as `/TD` (and fails 7.1-7);
  `/Alpha → /P → /Zed` types as `/P`; `/TR → /Zed → /TR` is a cycle (the revisit is asked first).
- **7.1 t5's subject is every untyped element and its failure is narrower**: only a non-standard `/S` that
  dead-ends at another non-standard name. A cycle passes it; `/Document → /Book` passes it (7.1-t07-fail-a).
Deep-dive did not fire: two rules through the registry pattern `/pending 548` used; no wire format or schema.
- T01 — the checker's own §14.8.4 set (`standardtypes.go`, 49 types), pinned against the table.
- T02 — `standardType` stops at the first standard type reached by mapping; `roleMapCircular` walks the raw map.
- T03 — register `7.1 t5` and `7.1 t7`, element-scoped; oracle documents for both failed halves; `corpusReach`
  rows (t5 **5**, t7 **294**); the count claims 20 → 22 at every site that states them.
- T04 — `pdfops.standardRole` gets the same algorithm (the plan's note: it shared the misreading).

#### P03.S02 — the containment matrix, one door *(done 2026-09-22, v1.140.0)*
Scope: a declarative table of (standard type → permitted parents, permitted kids) plus one generator that
registers a rule per clause from it. Lands `7.2 t3-t10`, `t17-t20`, `t26`, `t27`, `t36-t38` — seventeen
rules over tables, lists and TOC. Each keeps its own `Clause`, `Summary` and `corpusReach` row; the matrix is
the shared rule, not a shared verdict. Refs: law 1, ADR-009.
Acceptance:
- The matrix is the only place a containment relation is written; a guard asserts every registered
  containment clause routes through it, not that seventeen messages agree.
- **A pass fixture for every one of the fourteen *(pin, P03 phase close: thirteen by the phase note's own list — t4-t10,
  t18-t20, t36-t38)* clauses whose corpus is fail-only**, and a red proof that an
  always-Fail implementation is caught by it.
- Every clause's `corpusReach` row measured over the 297-file set.

**(grill, 2026-09-22)** veraPDF 1.30.2 was run on thirty `treeDoc` trees before the matrix was written, every
clause in both directions, and it settled every edge the relation's two predicates leave open: an UNTYPED kid
(a loop, a private dead end, a private self-map) is ignored, never refused; an untyped PARENT, and the structure
tree root, satisfy nothing; role-mapped names resolve first; MCIDs and object references are content, not kids;
an element with no element kids passes every "may contain only" clause. Two oracle documents carry all
seventeen clauses at once — every relation kept (17 pass) and every relation broken (17 fail).
Deep-dive did not fire: new rules through the registry; no wire format.
- T01 — `rules_containment.go`: the matrix (17 rows), `checkContainment` (the one door), `typedAs`.
- T02 — `treeDoc`, the nested-spec fixture builder; a measured pass and fail document per clause, plus the edges.
- T03 — the door guard (`TestContainmentIsWrittenOnlyInTheMatrix`) and the source-population guard taught the
  matrix's `clause:` fields.
- T04 — the two oracle documents; seventeen `corpusReach` rows measured; count claims 22 → 39.
- **Review (2026-09-22):** two confirmed false passes — the matrix read the tree walk's parents and kids, and
  veraPDF reads each element's own `/P` and `/K` — fixed, with object-by-object fixtures (`relationDoc`).

#### P03.S03 — cardinality and placement *(done 2026-09-22, v1.141.0)*
Scope: `7.2 t11-t14`, `t16`, `t28`, `t39`, `t40` — at most one `THead`/`TFoot`/`Caption`, a `TBody` required
when either is present, and `Caption` restricted to first or last kid. Counts and positions, which the
matrix deliberately does not express. Refs: law 1.
Acceptance:
- `t16`, `t28`, `t39` and `t40` have **no corpus file**: each gets a fixture and a recorded veraPDF verdict.
- ~~An element with no kids is `NotApplicable`, never a silent Pass.~~ **(pin, S03 grill, 2026-09-22 — measured)**
  veraPDF 1.30.2 counts a check on an empty `Table` and PASSES it for all six Table clauses, so an element with no
  kids is **Pass**; `NotApplicable` would disagree with the oracle on every such document. `NotApplicable` is kept
  for a document with no element of the clause's subject type, which is veraPDF's 0-check answer. "Silent" does not
  apply: the verdict is the oracle's, measured, not a default.

**(grill, 2026-09-22)** veraPDF was run on eighteen `treeDoc` trees before a rule was written, every clause both
ways — t16, t28, t39 and t40 have no corpus file, so these ARE their oracle. The one open edge, settled: an UNTYPED
kid is dropped from the sequence, never an empty slot — a Caption after an untyped first kid is first, and a second
leading Caption is in the middle (fails t16 as well as t39). Deep-dive did not fire.
- T01 — `rules_kidsequence.go`: eight rows over the typed-kid sequence, `checkKidSequence` the one door, reading
  S02's `elementKids`/`typedAs`.
- T02 — both-ways and edge tests; the one-door guard; the source-population guard taught `[]kidSequence`.
- T03 — the oracle's "every count and position broken" document (8 of 8 fail on veraPDF); eight `corpusReach` rows;
  count claims 39 → 47.
- **Review (2026-09-22):** a crafted `/Type /MCR` carrying `/S` counted as a kid; veraPDF reads it as content
  (measured). The `/Type` test S02 had removed as dead is restored and pinned.
- **(pin, 2026-09-22, v1.141.1 — found reading veraPDF's own table code for S04)** S02 and S03 shipped without
  veraPDF's pass-through tags: kids and parents are read THROUGH `NonStruct`, `Div` and `Part`. Measured on 1.30.2:
  `Table(Div(TR(TD)))` passes 7.2-3/7.2-4 (nib failed both), and a second `THead` in a `Div` FAILS 7.2-11 (nib
  passed). The corpus holds none of these shapes. Fixed in `elementKids`/`significantParent`, pinned by
  `TestNonStructDivAndPartAreLookedThrough`.

#### P03.S04 — table geometry: spans, and the grid nib does not reproduce *(done 2026-09-22, v1.142.0)*
Scope: `7.2 t15` (cells shall not intersect), `t41`, `t42`, `t43` (rows and columns agree once spans are
counted). The hardest four, and the one place the checker already **declares** a gap —
`rules_semantic.go:182` returns `CannotCheck` for a grid it does not build. Either the grid arrives here and
that `CannotCheck` retires, or it stays and this slice says which rules it costs. Refs: law 1, law 4.
Acceptance:
- `t42` and `t43` land together — identical wording, opposite polarity.
- Where the grid cannot be built the verdict is `CannotCheck` naming why, never `Pass`; the existing
  `reach_test.go` shape covers it.

**(grill, 2026-09-22)** The grid ARRIVES, and the `CannotCheck` retires. The rule measurement had not found is in
veraPDF's own source (`GFSETable.checkTable`, veraPDF-validation): lay the table out left to right into the next free
slot, stop at the FIRST irregularity (overflow → t42, a RowSpan past the height → t41, an occupied slot → t15, a short
or empty row → t43), and only for a regular table ask headers — every TH scoped, or else the FIRST data cell without
valid Headers is flagged (t1 when it named none, **t2** when it named unknown ones). Ported step for step
(`rules_table.go`), with one measured divergence from that source: the integration branch disables the geometric header
search for UA-1, and the installed 1.30.2 runs it (`7.5-t01-fail-c.pdf` agrees only with it on). Where veraPDF's Java
throws — measured: a RowSpan of 0 makes 1.30.2 raise ArrayIndexOutOfBoundsException — nib answers `CannotCheck` naming
why. Reading that source also found S02/S03's pass-through defect, fixed separately (v1.141.1).
- T01 — `rules_table.go`: the layout (memoised per Table), veraPDF's typed attribute reader (`/A` then ClassMap).
- T02 — t15, t41, t42, t43 registered; 7.5 t1 re-implemented over the layout (the heuristic in `rules_semantic.go`
  retired, its eleven `CannotCheck` expectations now veraPDF's verdicts), **7.5 t2 landed here** (see S06's pin).
- T03 — `treeDoc` cell attributes; both-ways and step-for-step tests, every row measured; two oracle documents.
- T04 — `corpusReach`: five rows at 36, and 7.5 t1 **27 → 36**; five `knownCannotCheck` rows retired; counts 47 → 52.
- **Review (2026-09-22):** a crafted span OOM-killed the process — veraPDF truncates its counts to 32 bits, and the
  port now does too, with a 4M-slot cap answering CannotCheck; an empty-string Header and an empty-name Scope read the
  way veraPDF reads them. All measured on 1.30.2.

#### P03.S05 — strongly or weakly structured, but not both *(done 2026-09-22, v1.143.0)*
Scope: `7.4.4 t1` (at most one child `H` per node), `t2` and `t3` (a document uses `H` or `Hn`, never both).
Interacts with the implemented `7.4.2 t1`, whose `corpusReach` is 134 — the widest in the family — so a
change to heading handling is measurable immediately. Refs: law 1.
Acceptance:
- A document using both `H` and `Hn` fails t2 and t3; one using neither is `NotApplicable`.
- `7.4.2 t1`'s `corpusReach` of 134 does not move, or the change is explained in the same edit.

**(grill, 2026-09-22)** veraPDF writes t2/t3 as profile VARIABLES (`usesH` set by any SEH, `usesHn` by any SEHn),
which read as traversal-order dependent. Measured on fourteen trees, every order and nesting: they are not — a
document with both kinds fails every H on t2 and every Hn on t3; one kind alone passes its clause and leaves the
other without a subject. t1 is every element's (PDStructElem) typed kids, pass-through tags looked through: `Div(H),H`
fails. Deep-dive did not fire.
- T01 — `7.4.4 t1` as a kid-sequence row over every element (`subject: ""`); t2/t3 in `rules_headings.go`.
- T02 — `TestHeadingStructureAgreesWithVeraPDF` (eight measured trees, three clauses each); two oracle documents.
- T03 — `corpusReach` t1 294, t2 7, t3 134; **7.4.2 t1 stays at 134** (the acceptance, measured); counts 52 → 55.

#### P03.S06 — notes, the Form element, and the parent entry *(done 2026-09-22, v1.144.0)*
Scope: the residue — `7.9 t1` and `t2` (`Note` has an `ID`, and IDs are unique), `7.18.4 t2` (a `Form`
omitting `Role` has exactly one object-reference child), `7.1 t12` (every element carries `/P`), `7.5 t2`
(the sibling of the implemented `t1`). Refs: law 1.
Acceptance:
- `7.1 t12` and `7.18.4 t2` have no corpus file; both get a fixture and a recorded veraPDF verdict.
- `7.9 t2`'s uniqueness is checked across the whole tree, not per subtree.
- **(pin, 2026-09-22, S06 grill — measured) `7.1 t12` is NOT implemented, and that is the finding.** veraPDF 1.30.2
  PASSES 7.1-12 on an element with no /P, with /P null, /P 5, a dangling /P and a /P naming a page — the installed
  validator gives an element the parent it was reached from. So the clause's failed half cannot be reached by any
  document for an element reached through the tree, and a rule nib registered could only ever pass: the trap this
  phase's opening note names. The acceptance "gets a fixture and a recorded veraPDF verdict" is met — five fixtures,
  five recorded passes — and the rule stays unregistered, declared. Revisit on a veraPDF upgrade.
  **(pin, 2026-09-22, P03 phase close — the explanation above is withdrawn, the verdicts stand)** "gives an element
  the parent it was reached from" was an inference, and S02's measurement contradicts it as a general reading:
  veraPDF's containment clauses read the element's own `/P` (a TD whose `/P` names the Document fails 7.2-9). Why
  7.1-12 passes all five shapes is unmeasured; the five recorded passes are the finding, and no rule may borrow the
  mechanism.
- **(grill, 2026-09-22)** 7.9 and 7.18.4 t2 ported from veraPDF's source (`GFSENote`, `GFSEForm`, veraPDF-parser
  `TaggedPDFHelper.getChildren`) and measured on twelve fixtures: an empty Note ID is no ID and still collides;
  the ID set is the document's; a Form's children are every /K entry, MCIDs included; a Role attribute excuses it.
  (pdfcpu's reader rewrites an annotation carrying field keys to /Widget, so the "not a widget" fixture is a Text note.)
  T01 `rules_notesform.go` (+ `attributeOfType`, the owner-general attribute door); T02 the measured tests and four
  oracle documents; T03 `corpusReach` 7.9 t1/t2 9, 7.18.4 t2 18; counts 55 → 58.
- **(pin, 2026-09-22, P03.S04)** `7.5 t2` landed in S04, not here: it is the other half of the same veraPDF
  computation as 7.5 t1 (`hasConnectedHeader` with `unknownHeaders` set), so it shares the layout rather than
  re-deriving it. S06 keeps `7.9 t1`/`t2`, `7.18.4 t2` and `7.1 t12`.

### P04 — Checker: language (~10 rules) *(done 2026-09-23, v1.148.1)*
**Goal.** Outline entries, ActualText/Alt/E, annotation Contents, form TU and marked-content spans.
**Exit criteria.** As P03.

**(phase-open, 2026-09-22, v1.144.1)** The family is **ten** rules, read off veraPDF 1.30.2's own profile
(`PDFUA-1.xml` in the CLI jar): `7.2 t2` (PDOutline), `t21`–`t23` (PDStructElem ActualText/Alt/E), `t24` (PDAnnot
Contents), `t25` (PDFormField TU), `t29` (CosLang: every `/Lang` value matches `^[a-zA-Z]{1,8}(-[a-zA-Z0-9]{1,8})*$`),
`t30`–`t32` (SEMarkedContent Span ActualText/Alt/E). **Pin — `7.1 t1` and `7.1 t2` join this phase**: artifact inside
tagged content and tagged content inside an artifact are SEMarkedContent rules over the same marked-content stack
`t30`–`t32` read, and no phase of this plan owned them (P03's family was 43 rules without them; P06's file-level
list does not name them). Twelve rules, so P04 lands the checker at **70 of 106**.

- **Every one of the language rules rests on one global, `gContainsCatalogLang`**, and nib already answers the
  neighbouring question for 7.2 t33/t34 (`declaresLang`, "present counts, even empty"). Whether veraPDF's global
  means *present*, *non-empty* or *a valid identifier* is the first thing measured — it decides every rule's pass
  half — and it gets ONE door (ADR-009), not twelve inline reads.
- **The veraPDF predicates differ in which inheritance they allow**, and the difference is the trap: a structure
  element's alternates may take `parentLang` (an ancestor's `/Lang`); an annotation's Contents and a field's TU may
  NOT — only `containsLang` or the catalog; a Span's property list takes `inheritedLang`. What `containsLang` means
  for an annotation and a field is read from veraPDF's source before a rule is written (GFPDAnnot, GFPDFormField),
  then measured on 1.30.2 — the method P03 used.
- **Corpus files by NAME** (the specification's numbering, which P03.S01 found is not veraPDF's — read the clause off
  a veraPDF run): t29 has 26 files (10 pass, 16 fail), each other clause two or three pass and one fail. Every clause
  owes a measured pass fixture of its own regardless (P03's phase trap).
- **7.2 t29's object is every `/Lang` in the file** — catalog, structure element, marked-content property list, and
  whatever else veraPDF's CosLang population reaches; its population is measured, not assumed from the description.

#### P04.S01 — the catalog language, and every language identifier *(done 2026-09-22, v1.145.0)*
Scope: `gContainsCatalogLang` as one door, measured on veraPDF 1.30.2 (absent, empty, invalid, indirect); `7.2 t2`
(an outline requires it); `7.2 t29` over veraPDF's CosLang population, with the population measured. Refs: law 1,
ADR-009.
Acceptance:
- One predicate answers "the catalog determines a language", and every P04 rule reads it; a guard asserts the routing.
- `7.2 t29`'s population matches veraPDF's on measured fixtures for each `/Lang` holder, and its `corpusReach` row is
  measured over the 297-file set.
- `7.2 t2` and `t29` each hold a measured pass and fail fixture.

**(grill, 2026-09-22)** Read from veraPDF's source (veraPDF-validation integration: `GFPDDocument`, `GFPDStructElem`,
`GFPDAnnot`, `GFPDFormField`, `GFOpMarkedContent`, the profile's `<variables>`), then measured on 1.30.2 over
twenty-three fixtures before a rule was written:
- **`gContainsCatalogLang` is "the catalog's `/Lang` is a string"** — empty `()` counts, an indirect string counts, a
  name does not. That is exactly `catalogDeclaresLang`, which 7.2 t33/t34 already read: it becomes the one door.
- **7.2 t2**: an outline item exists (veraPDF's `PDOutline` objects) and the catalog door is false → fail; no item →
  no subject. Measured: absent fails, `()` passes, `/en` fails, indirect passes.
- **7.2 t29's population** is every STRING `/Lang` on: the catalog; every structure element; the element an
  annotation's or a field's `/StructParent` names (through the parent tree); every BDC or DP property list, inline or
  named in `/Properties`. A non-string `/Lang` is not a subject. The test is the regex over the decoded value: `()`
  fails, `en_US`, `en-`, a 9-letter subtag, `1en`, `en US` fail; `x-klingon` and a UTF-16 `en-US` pass. A property
  list defined in `/Properties` but never used is not a subject.
- **veraPDF reads marked content inside a USED tiling pattern and a Type 3 glyph procedure** (measured fail on both;
  an unused pattern passes). nib's content walk enters neither. So a failing `/Lang` found only in such a stream is
  **CannotCheck** (nib cannot tell used from unused without walking it), and a valid one changes nothing.
  **(pin — S04 gains this)**: walking used patterns and Type 3 procedures belongs to the slice that owns the
  marked-content stack, because it changes every content rule's events.
- A name-typed `/Lang` on the catalog or a structure element makes nib unable to read the file at all (pdfcpu's
  validator) — the class `/pending 612` names; recorded there, not fixed here.
Deep-dive did not fire: two rules through the registry; no wire format or schema.
- T01 — `catalogLang` door (value and presence); `catalogDeclaresLang` calls it; a guard that the catalog's `/Lang` is
  read nowhere else.
- T02 — `7.2 t2` over the outline's first item.
- T03 — `7.2 t29`: the population above, the content walk recording BDC/DP property-list `/Lang` values, the
  unwalked-stream scan answering CannotCheck.
- T04 — measured tests both ways; oracle documents; `corpusReach` rows; count claims 58 → 60.
- **Review (2026-09-22):** two false passes, both measured on veraPDF — a Type 3 font written directly in `/Resources`
  and a form drawn only from inside a pattern were read by nobody. The unwalked scan now walks the resource graph
  instead of the object table, and since pdfcpu's validator DROPS a direct Type 3 font (raw parse keeps it), a missing
  font entry asks the raw file once and answers CannotCheck (a genuine `null` — two corpus files — does not). The
  walk's `/Lang` record keeps a count and the first failure only (it had held 1.6 GB), and non-drawing operators got
  the content walk's third budget (`maxContentOperators`). The outline's `/First` test is unreachable through pdfcpu
  (it drops such an `/Outlines`), kept as veraPDF's subject definition. Live: `nib ua` on the fixtures and on nib's
  own `--lang en-US` README conversion (veraPDF PASS).

#### P04.S02 — a structure element's alternate text has a language *(done 2026-09-23, v1.146.0)*
Scope: `7.2 t21`, `t22`, `t23` — ActualText, Alt and E on a structure element, determined by its own `/Lang`, an
ancestor's, or the catalog's. One table over the three keys, one door. Refs: law 1, ADR-009.
Acceptance:
- The three clauses are one relation with three rows; a guard asserts no clause is written outside it.
- `parentLang`'s reach (which ancestors count; a pass-through element; a `/P` loop) is measured, and an unfinished
  climb is CannotCheck, never Pass (P03's phase-close lesson).
- Each clause holds a measured pass and fail fixture and a `corpusReach` row.

**(grill, 2026-09-22)** Read from veraPDF's source — `GFPDStructElem.getparentLang`/`getAlt`/`getActualText`/`getE`
in veraPDF-validation, and `PDStructElem.getLang`/`getParent`, `COSDictionary.getStringKey` and `COSName.getString`
in **veraPDF-parser** (the classes are in the parser repo, not the library one) — then measured on 1.30.2 over
**80 fixtures** before a rule was written. The predicate is
`<key> == null || containsLang || parentLang != null || gContainsCatalogLang`, and five of its six premises moved:

- **The subject is EVERY structure element, not every element carrying the key.** veraPDF runs one check per
  `PDStructElem` and passes it when the key is absent, so a tagged document with no `/Alt` anywhere reports
  `7.2 t22` **passed with checks**, not "no subject". nib therefore answers `Pass` whenever the tree holds an
  element, and `NotApplicable` only when it holds none. Measured both ways.
- **The three keys are not read alike.** `Alt` and `E` go through `getStringKey`, which returns **null for a name**;
  `ActualText` goes through `getKey(…).getString()`, which has no such exclusion, so **a name-typed `/ActualText`
  IS a subject and fails** (measured: `/ActualText /a` → failed, `/Alt /a` → passed). It does not reach nib: pdfcpu's
  validator refuses a name-, number-, boolean-, array- or dictionary-typed value on all three keys, so **in a
  document nib can read, each key is a string literal, a hex string, an indirect string, or `null`** (33 types
  measured). `d.text` is exactly that population, and the name asymmetry is unreachable — declared, not coded.
- **An empty string is a subject.** `/Alt ()` fails with no language, and `/ActualText ()` too — `getString()`
  returns `""`, not null. (Contrast 7.3 t1, which wants a NON-empty `/Alt`.)
- **An empty `/Lang` satisfies.** `()` on the element, on an ancestor, or on the catalog all pass — the same
  "present and a string" door `catalogDeclaresLang` already is, now asked of any dictionary.
- **The climb is blind, and it does not stop at the tree.** `getParent()` wraps whatever `/P` names and reads its
  `/Lang`, so a `/Lang` **on the StructTreeRoot** satisfies the rule (measured, at one and two levels), as does a
  `/P` naming a **non-ancestor element** or a **plain non-element dictionary** that carries one. Nothing is skipped
  either: a `Div` ancestor's `/Lang` counts, which is the OPPOSITE of `significantParent`, whose whole job is to
  climb past pass-throughs. So this is a second, different climb — not a reuse of the containment door.
- **A cycle is a Fail, not a CannotCheck.** veraPDF seeds the loop guard with the element's own key and returns null
  on a revisit: `/P` naming itself fails, and a true two-element cycle with no `/Lang` anywhere fails. That is a
  complete answer — every ancestor was seen and none carried a language — so nib fails it too, where
  `significantParent` reports "parent unreadable". Measured on both shapes.
- **The chain is unbounded in the input and readable.** The tree itself cannot be deep (pdfcpu refuses a structure
  tree past 100 levels — measured, a 120-deep tree is unreadable), but a **sideways** `/P` chain hanging off one
  shallow element is not the tree: **5,000 links is readable and veraPDF passes it.** So the climb gets a document-wide
  step budget, and a climb that spends it is `CannotCheck`; a refusal is never memoised (P03's kid-depth lesson —
  an answer must not depend on which element asked first), while a found language and a definitively-exhausted
  chain are, which makes the pass linear in distinct dictionaries.
  **(pin — built as a PER-CLIMB depth bound, not a document-wide step budget.)** With the memo the total work is
  already linear in distinct dictionaries, so a second document-wide budget bought nothing a bound does not, and a
  budget large enough for a real tree (one corpus file holds 12,711 elements) is one no test can reach — the
  untestable-branch defect P03's review filed twice. The bound is `maxWalkDepth + 1`: the `+ 1` is the
  StructTreeRoot, which the tree walk never counts because it starts below it, and without it the climb refused an
  element the walk had admitted (measured — a 65-deep tree whose only `/Lang` is on the root answered CannotCheck
  where veraPDF passes).
- An element reachable **only through the parent tree** is not a subject (measured: `/Alt` on one, `t22` passed) —
  the population is `structNodes`, which is already that set.

Deep-dive did not fire: three rules through the registry, no wire format, no schema, and the one seam it touches
(`structNodes`) is this plan's own P03 code.
- T01 — `parentLang` door: the blind `/P` climb, veraPDF's loop seeding, the bound above, the memo that never
  caches a refusal and carries its distance so a deeper climb cannot skip the bound.
- T02 — the three clauses as ONE relation over a three-row table, plus the guard that no clause is written outside it.
- T03 — measured tests both ways per clause (own/ancestor/root/catalog/empty/cycle/no-`/P`/budget), and the
  `NotApplicable`-only-when-no-elements half.
- T04 — oracle documents, `corpusReach` rows, count claims 60 → 63 (README, `docs/accessibility-parity.md`, and
  two prose copies in `rules_catalog.go` and `pdfops/labelua.go` that no guard reads — found stale at 60).
- **Review (2026-09-22):** one CRITICAL outside the slice's own code — `d.text` read a reference to a FREE object
  as an empty string that is present, which made nib FAIL 7.2 t22 on a dangling `/Alt` (veraPDF: no subject) and
  PASS it on a dangling `/Lang` (veraPDF: fail); fixed at the door, no corpus row moved. The bound gained its
  `+ 1` (above), the catalog now settles the clause before the walk, the bound's VALUE is tested on the boundary
  rather than 60-against-70, and the one-door guard asserts the three checks are ONE function rather than three
  that agree. **`/pending 635` filed and named at both sites**: `declaresLangFor` (7.2 t34) is a second climb for
  an overlapping question and it FAILS a document veraPDF passes when the only `/Lang` is on the StructTreeRoot.
  Live: `nib office` + `nib tag edit` produced both halves through real product doors and veraPDF agreed on both.

#### P04.S03 — an annotation's Contents and a field's TU have a language *(done 2026-09-23, v1.147.0)*
Scope: `7.2 t24` (annotations, excluding what veraPDF excludes) and `7.2 t25` (form fields), with `containsLang` read
from veraPDF's source and measured — no inheritance from ancestors unless veraPDF grants it. Refs: law 1.
Acceptance:
- `containsLang` for an annotation and for a field is stated from veraPDF's source and measured on 1.30.2.
- Each clause holds a measured pass and fail fixture and a `corpusReach` row.

**(grill, 2026-09-23)** The predicates, read off veraPDF 1.30.2's own profile: t24 is
`Contents == null || containsLang == true || gContainsCatalogLang == true` and t25 is the same with `TU`. **The
slice's finding is what `containsLang` turned out to be**, and it is not the shape the sketch implies: it is not a
key on the annotation or the field at all, and it is not an ancestor climb. `GFPDAnnot.getLang`
(`GFPDAnnot.java:245-259`) and `GFPDFormField.getLang` (`GFPDFormField.java:93-105`) take the holder's
`/StructParent`, resolve it through the StructTreeRoot's `/ParentTree`, and read that ONE element's own `/Lang`,
requiring a string. **There is no `/P` walk** — so the question the phase-open note left open ("no inheritance
from ancestors unless veraPDF grants it") is answered: veraPDF grants a ParentTree ASSOCIATION, which is a
different mechanism from P04.S02's `parentLang`, and reusing that climb would pass documents veraPDF fails.

**Populations, from veraPDF's source rather than from the description.** `GFPDPage.parseAnnotations` builds a
`PDAnnot` for every entry of a page's `/Annots` and filters nothing — `createAnnot` switches on the subtype only
to choose a subclass, and every subclass IS a `PDAnnot` — so the scope line's "excluding what veraPDF excludes"
resolves to **nothing is excluded**. `GFPDAcroForm.getFormFields` takes the AcroForm's `/Fields` and
`GFPDFormField` exposes `/Kids` as linked objects, so nested fields are subjects too.

**(grill) The traversal already existed and was widened rather than duplicated.**
`structParentsOfAnnotsAndFields` (P04.S01, for 7.2 t29) already walked every page annotation and every AcroForm
field through `/Kids` and resolved `/StructParent` — but yielded only the resolved ELEMENT, discarding the holder
that carries `/Contents` and `/TU`, and dropping the subjects whose `/StructParent` resolves to nothing, which
are exactly t24's and t25's failures. It is now `annotAndFieldSubjects`, one traversal with two readers
(ADR-009); t29's reader is a filter over it and its corpus reach is unchanged.

Tasks:
- T01 — widen the traversal to `annotAndFieldSubjects` (holder, where, resolved element, population kind); keep
  `structParentsOfAnnotsAndFields` as t29's filter over it, byte-identical in behaviour.
- T02 — `associatedTextKeys` + `checkAssociatedTextLanguage`: one relation, two rows, no climb.
- T03 — measured fixtures: the oracle gains an annotation carrying `/Contents` whose named element declares no
  `/Lang` (t24's failing half, which no corpus document reached); `corpusReach` gains 7.2 t24 at 71 and t25 at 19.
- T04 — the no-climb property asserted directly, with its near control, and red-proved against a climbing
  implementation.
- T06 — the slice's own diff review, worked to zero: it found the verdict logic collapsing two different
  facts into one document-wide string — a subject whose parent-tree slot nib never read, and a population
  that may be short of members — so a deep parent tree anywhere downgraded every definite failure in the
  file to CannotCheck, across BOTH populations. Split into `langSubject.unresolved` and
  `subjectScan.annots`/`.fields`; two new gap-down tests, each red-proved. The review also found `kindOf`
  deriving the population from the clause id with an `else` that would have made any third row a form-field
  rule, under a comment claiming that was impossible — the kind is now a stated field with a row-by-row guard.
- T05 — the count claim: 63 → 65 in the README and the parity doc, and the two prose copies the P04.S02
  inventory recorded as having NO reader (`rules_catalog.go`, `pdfops/labelua.go`) gain one.

**(grill) What the slice found in code it did not write.** The ancestor fixture reports **7.2 t34: veraPDF
passed, nib failed** — a live false FAIL in a shipped clause, and a SECOND reproduction of `/pending 635` that is
not its StructTreeRoot special case. It is filed there with the fixture name rather than fixed here: fixing t34
is that item's work, and putting the document in the oracle corpus would make every later slice red until it is
done. 7.2 t24 itself AGREES with veraPDF on that document, which is what measures the no-climb finding.

#### P04.S04 — marked content: Span alternates and artifact nesting *(done 2026-09-23, v1.148.0)*
Scope: `7.2 t30`, `t31`, `t32` (a Span property list's ActualText/Alt/E, determined by its own `/Lang`, an inherited
one, or the catalog) and `7.1 t1`, `t2` (an Artifact sequence inside tagged content; tagged content inside an
Artifact), over the content walk's marked-content stack. **(pin, P04.S01 grill)** veraPDF reads marked content inside
a used tiling pattern and a Type 3 glyph procedure, and nib's content walk enters neither — this slice decides whether
the walk enters them (and what that does to 7.1 t3's and the font rules' events), and retires S01's CannotCheck for
7.2 t29 if it does. Refs: law 1, law 4.
Acceptance:
- `inheritedLang` and `isTaggedContent` are stated from veraPDF's source and measured, including inside form XObjects
  and annotation appearance streams.
- A content walk that stopped (P03's budgets, an unreadable stream) is CannotCheck for all five, never Pass.
- Each clause holds a measured pass and fail fixture and a `corpusReach` row.

**(grill, 2026-09-23)** Read off veraPDF 1.30.2's profile and its `validation-model` source, then measured over
**47 purpose-built fixtures** and the corpus's own nine, before a rule was written. The predicates are
`tag != 'Artifact' || isTaggedContent == false` (7.1 t1), `isTaggedContent == false ||
parentsTags.contains('Artifact') == false` (7.1 t2) and, for t30/t31/t32, `tag != 'Span' || <KEY> == null ||
Lang != null || inheritedLang != null || gContainsCatalogLang == true`.

- **The subject is one balanced BMC *or* BDC sequence, at every depth** (`GFPDSemanticContentStream.java:109`,
  `GFSEMarkedContent.java:95`): the object is added in the `EMC` branch, so an **unbalanced** sequence produces
  no subject at all — its operators fall to `GFSEUnmarkedContent`. Measured: a `BDC` with no `EMC` yields one
  subject, not two. **`tag` is the name without its slash**, and it is `arguments[size-2]` for BDC against the
  *last* argument for BMC (`GFOpMarkedContent.java:108-116`, `GFOp_BMC.java:57-65`) — so a malformed
  `/Artifact BDC` written with no property list has a **null** tag and breaks no rule. Three of this slice's
  first-round fixtures were that shape and measured the wrong thing; the round that fixed them is what produced
  the table below.
- **`isTaggedContent` is NOT "an MCID is present"** (`GFSEGroupedContent.java:145-163`). It resolves the
  sequence's struct element — its own `/MCID` through the `/ParentTree`, else the one inherited from the
  enclosing sequence — and climbs `/P` asking whether the chain **reaches the StructTreeRoot**. An MCID naming
  a slot the parent tree does not hold, and an element detached from the root, are both *untagged*.
- **The three inheritances differ, and that is the slice.** `parentsTags` includes the object's **own** tag
  (`GFOpMarkedContent.java:142-151`) and **crosses a form XObject boundary** into the invoking stream
  (`OperatorParser.java:538-550`). `isTaggedContent`'s struct element crosses it too. **`inheritedLang` does
  not** — its chain is per-content-stream (`OperatorParser.java:168,176`), so a `/Lang` in force on the page
  does not reach a Span inside a form the page draws. All three measured, in both directions.
- **`inheritedLang`'s order** (`GFOpMarkedContent.java:153-166`), stopping at the first hit: the sequence's own
  `/MCID`-resolved element with a `/P` climb; else the **enclosing** sequence's own property-list `/Lang`; else
  that enclosing sequence's `inheritedLang`. It never consults the page or the catalog — the catalog is the
  separate `gContainsCatalogLang` disjunct, which is `catalogDeclaresLang`, P04.S01's door.
- **`Lang`/`ActualText`/`Alt`/`E` come from the BDC property list only** — inline dictionary or a name resolved
  through `/Resources /Properties` (`GFOpMarkedContent.java:68-84`, `:203-211`). **BMC has no property list**,
  so t30-t32 can never fire on one. Measured: `/Span /MC0 BDC` with `/MC0` holding `/Alt` fails t31.

**(grill) The P04.S01 pin is REFUTED as stated, and the two populations it conflated are why.** veraPDF does
**not** read marked content inside a tiling pattern, a Type 3 glyph procedure or an annotation appearance:
`GFPDTilingPattern.java:99-103`, `GFPDType3Font.java:116` and `GFPDAnnot.java:465-473` each build a plain
`GFPDContentStream`, and `new GFPDSemanticContentStream` occurs at exactly two sites — the page
(`GFPDPage.java:250`) and a form XObject drawn from a semantic stream (`GFPDXForm.java:211`). Measured with the
page fully covered so only the nested stream could fail: a pattern's and a glyph's marked content moves neither
7.1 t3 nor 7.2 t30-t32 nor 7.2 t34. What veraPDF *does* read there is **`CosLang`**, t29's population — a
different object, and the one S01 actually measured. **So the walk does not enter them for these five rules,
and 7.1 t3's and the font rules' event counts do not move: the answer to the pin's parenthesis is `nothing`.**

**(grill) S01's CannotCheck is retired anyway, because the population turned out to be DRAWN rather than
DEFINED.** Measured: a bad `/Lang` in a pattern that is *defined in `/Resources` and never selected* passes
7.2 t29 with **zero** checks, and so does one in a Type 3 font no glyph is shown in, and one in a form no `Do`
invokes, and one in an object nothing references. `unwalkedBadLang` reads what is defined, which is why it could
only ever answer CannotCheck. The two entry conditions, both measured: a tiling pattern is read once
`scn`/`SCN` **selects** it — painting is not required — and a Type 3 font's **every** `CharProc` is read once
*any* glyph is shown in it (showing `/b` fails on `/a`'s bad `/Lang`), while selecting the font with `Tf` and
showing nothing reads none. So the content walk enters both, in a **lang-only** mode that emits no content
event and no marked-content subject, and t29 answers Fail or Pass where it answered CannotCheck.

**(grill) FOUR measured false passes in shipped rules, all downstream of the two predicates above.** Each is nib
`Pass` where veraPDF fails — `Verdict.conformant`, so each lets a non-conformant document be called conformant,
which is law 5's own failure mode. They are fixed here rather than filed, because both predicates are what this
slice is required to state from veraPDF's source and a rule gets ONE door (ADR-009):

| fixture | veraPDF | nib today | cause |
|---|---|---|---|
| `h_mcid_unresolved` | 7.1 t3 **fail** | pass | `covered` counts any `/MCID n`; the slot is not in the parent tree |
| `h_mcid_detached` | 7.1 t3 **fail** | pass | the element resolves but its `/P` chain never reaches the StructTreeRoot |
| `j_text_in_artifact_nolang` | 7.2 t34 **fail** | pass | nib exempts text inside an `/Artifact`; **veraPDF's test has no such disjunct** |
| `j_text_in_artifact_in_form` | 7.2 t34 **fail** | pass | the same, across a form boundary |
| `e_form_inside_mcid_lang` | 7.2 t34 **fail** | pass | the language crosses a form boundary; veraPDF's chain is per-stream |
| `e_form_inside_mcid_elemlang` | 7.2 t34 **fail** | pass | the same, in the structure-element spelling |

**(grill, found DURING implementation) The last two were found by reading the finished walk back, and they
are why 7.2 t34 now reads the one door.** The first four were measured before a line was written; these two
were not, because they are not about the marked-content SEQUENCE the slice set out to model — they are the
same boundary applied to a content ITEM, in a rule the slice was only editing. Both were already sitting in
the probe output, unread. **The boundary is the WALKER's stream and not the innermost frame's**: a form whose
own content opens no sequence has a stack of INHERITED frames only, every one carrying the invoking stream's
number, so comparing against the last frame inherits exactly what the rule forbids.

**And `/pending 635` closes here rather than separately, because the second climb WAS the defect.** t34
answered "is a language determined" with `declaresLangFor`, a second implementation that disagreed with
`parentLang` at the StructTreeRoot, at a `/P` cycle and by one on the bound — a live false FAIL in a shipped
clause, filed before this slice and amended by P04.S03 with a second reproduction. Leaving it would have
shipped two disagreeing implementations of the exact predicate this slice is required to state from veraPDF's
source, which is the ADR-009 violation 635 itself complains about. `declaresLangFor` is **deleted**, t34 reads
`inheritedLangOf`, and 635's own held-out fixture and its control are now oracle-corpus documents where
veraPDF measures both rules on them. The item was marked `/grill`-required on the ground that a shipped rule's
verdicts change; the evidence it asked for is the 297-file corpus, which is green.

Tasks:
- T01 — `isTaggedContent` as ONE door over the content walk's frames: resolve the innermost struct parent
  (own `/MCID`, else inherited across the form boundary), climb `/P` to the StructTreeRoot under a bound, and
  distinguish *definitely untagged* from *nib could not read the parent tree* — S03's `unresolved` split, which
  exists because collapsing them downgraded definite failures document-wide.
- T02 — the walk emits marked-content **subjects** beside its drawing events: one per balanced sequence, with
  tag, property-list `/Lang`/`/ActualText`/`/Alt`/`/E`, `parentTags` (own tag included, crossing forms) and the
  per-stream `inheritedLang` chain. Appearance streams emit events as now and **no** subjects.
- T03 — `7.1 t1`, `7.1 t2` and `7.2 t30`/`t31`/`t32` over that population; a stopped walk is CannotCheck for
  all five, and so is an `isTaggedContent` nib could not settle.
- T04 — 7.1 t3 reads the T01 door, closing the first two false passes; 7.2 t34 drops its `/Artifact` exemption,
  closing the other two.
- T05 — the lang-only walk into selected tiling patterns and shown Type 3 glyphs; `unwalkedBadLang` and
  `maxUnwalkedStreams` retire with the CannotCheck they served.
- T06 — fixtures: each of the five clauses gains a measured pass and fail fixture of its own, the four
  false-pass documents become standing gap-down tests, and `corpusReach` gains five rows.
- T07 — the count claim: 65 → 70 in the README and the parity doc, and the two prose copies still reading
  **60** (`uacheck.go:112`, `door.go:22`) — which P04.S02's and S03's own T05 missed — are corrected AND
  given a reader, so every copy of the number now has one. Two slices' T05 each said "the two prose copies"
  and each meant a different two, which is the argument for a reader rather than a third correction.
- T08 — 7.2 t34 reads the one language door: the stream boundary for a content item, and
  `declaresLangFor` deleted, closing `/pending 635` with its fixture promoted into the oracle corpus.
- T09 — the lang-only walk is memoised per STREAM. Found by reading the walk back: `enterType3` fires on
  every text operator and `enterPattern` on every `scn`, each spending a `maxFormWalks` unit, so an ordinary
  page of Type 3 text would have tripped the budget and turned EVERY content rule into CannotCheck.

**(acceptance ledger, 2026-09-23, v1.148.0)** Every clause split on `and`; nothing `not exercised`.

| # | clause | how it was discharged |
|---|---|---|
| 1 | `inheritedLang` stated from veraPDF's source | `GFOpMarkedContent.java:153-166` — struct-parent lang, then the enclosing sequence's own `/Lang`, then recurse; per-content-stream |
| 2 | `isTaggedContent` stated from veraPDF's source | `GFSEGroupedContent.java:145-163` — resolve the struct parent, climb `/P`, ask whether it reaches the StructTreeRoot |
| 3 | and measured | 47 purpose-built fixtures over five rounds + veraPDF's own nine + the 297-file corpus |
| 4 | including inside form XObjects | `TestInheritedLanguageDoesNotCrossAFormBoundaryWhileTheArtifactDoes`, `TestTheContentLanguageStopsAtTheStreamBoundary` — both directions, each with its control |
| 5 | and annotation appearance streams | `TestAnAppearanceStreamIsNoMarkedContentSubject` — measured on 1.30.2, one sequence counted where the document holds three |
| 6 | a stopped content walk is CannotCheck for all five, never Pass | `TestAStoppedContentWalkIsCannotCheckForAllFive`, with a two-level control; `TestTheMarkedContentRulesAreCannotCheckWhenTheTreeRunsOut` for the tree half |
| 7 | each clause holds a measured pass fixture | the oracle's four new documents + the unit tables |
| 8 | and a fail fixture | the oracle reported all five clauses' failing half unreachable by name until those documents were added |
| 9 | and a `corpusReach` row | 7.1 t1/t2 and 7.2 t30/t31/t32 at **293** each; pairs 19,137 → 20,612 |

**Gates.** `suiterun OK` fp `47f7cb06a46c`, 7m33s — `go build ./...`, `go test ./...` (24 packages, 0 FAIL),
jsdom **454/454**, uirepro **160/160**. Corpus **0 false pass, 0 false fail**. `/redproof`: ten mutations, one
condition each, **no survivors**, plus three probed during implementation. **Tiers 4 and 6 did NOT fire** — the
slice touches neither `internal/server`'s session, ceremony, delivery or discovery paths, nor `internal/p2p`,
nor `internal/rendezvous`; the whole diff is `internal/uacheck` plus two comment repoints in `internal/pdfops`.

**What the slice cost beyond its scope, and why it was in scope anyway.** Five new rules were the smallest part
of it. Reading each predicate off veraPDF's source and measuring it before writing a line turned up **six false
passes in three shipped clauses** — 7.1 t3 twice, 7.2 t34 four times — and one **false FAIL** (`/pending 635`)
that a second implementation of "is a language determined" had been causing since P07.S02. Every one is a rule
reporting a document conformant that veraPDF does not, which is what law 5 exists to catch and what the corpus
could not see: no corpus file has a dangling MCID, and none has its only unlanguaged text inside an `/Artifact`.

**(phase close, 2026-09-23, v1.148.1)** Exit criteria are P03's, split and each discharged:

| # | clause | how |
|---|---|---|
| 1 | every rule in the family passes `veracorpus_test.go` | all **twelve** carry a `corpusReach` row and the corpus is **0 false pass, 0 false fail** over 20,612 pairs |
| 2 | rules without corpus files agree with veraPDF on fixtures of their own | the oracle names any clause whose failing or passing half no document reaches; it names none — both halves of all twelve are reachable, which is the stronger form of the clause |
| 3 | *(phase-open pin)* twelve rules land the checker at 70 of 106 | `len(Clauses())` **= 70**, run rather than counted |

**Required-run gates, enumerated rather than assumed.** The project's gate is: did the work touch
`internal/server`'s session, ceremony, delivery or discovery paths, `internal/p2p`, or `internal/rendezvous`?
Across P04's four slice commits (`b527bcf`, `f71c428`, `27020aa`, `dcf16da`) — **no**, so **tiers 4 and 6 did
not fire for this phase**. The range `b527bcf~1..HEAD` *does* show `internal/p2p/attestation.go`, and it belongs
to `ee1100c`, the `/pending 613` signature fix that landed interleaved between S02 and S03; that commit fired
both gates and recorded them (tier 4 pairrepro over both transports, tier 6 ceremonyrepro 28/0). Checked by
attributing the path to its commit, not by reading the range's file list.

**The cross-slice review is what this phase close was for, and it found what a slice review structurally
cannot.** Nine findings over the four slices' combined code. The one that mattered: **S04 asked the catalog
short-circuit THIRD while every other language clause asks it FIRST** — S02 and S03 had written that order
deliberately and documented why — so on a document with a catalog `/Lang` and a walk that could not finish,
7.2 t30/t31/t32 refused while t21-t23, t24/t25, t33 and t34 all passed. Fixed; **corpus reach rose 293 → 295**
on those three, two files moving from refused to settled. Five further real defects: two `/P` climbs charging
one bound an ancestor apart under a comment claiming they were identical; the inline-image path spending a
budget unit without consulting the ceiling; `scanInlineType3` turning "nib did not look" into "there is none";
a parent-tree slot present-but-unreadable reported as "names no element"; and a comment S04 falsified.

**Two reported majors were REFUTED or INVERTED by measuring instead of reasoning**, and both are recorded as
tests rather than as fixes: a `/Lang` on the **StructTreeRoot** is accepted by the climb and never graded by
7.2 t29 — veraPDF does not grade it either (zero checks) — and an **appearance stream's** BDC `/Lang` IS graded
by t29 although that sequence is no `SEMarkedContent`. nib already matched the oracle on both; what was missing
was the measurement.

**The phase-close gate went RED once, and the cadence datum it owes is: the per-commit tier WOULD have caught
it.** `TestEveryDocCommentNamesItsOwnFunction` (repo root, `/pending 352`) fired on a doc comment added during
this close's own fix pass — Go binds a doc block to the function below it, so a block not opening with that
function's name leaves it undocumented and reads as a paragraph about its neighbour. The targeted run used
during the fix pass was `go test . -run Parity`, never `go test .` whole, and the whole-package run is part of
the per-commit tier. So this instance argues for running the cheaper tier more often, not the full suite.

**Graduation pass:** 56 rows, **53 keep-live mechanically**, **3 needing judgement**, **0 hot-path**. One
graduated — S03's row S11 described `/pending 635` and its own note said "whoever fixes it adds them"; S04 did,
so the row now names a standing reader instead of a parked defect. Nothing deleted, nothing gated. Recorded in
`instruments/ua-coverage.md`; `inventorycheck` exit 0, 253 readers resolve.

**Pending sweep against the closure:** no item in the `Phase` section is gated on a P04 coordinate (457 waits
on P05, 493 on P07), so the closure falsifies none of them — the sweep ran and found nothing, which is not the
same as not running. `/pending 612` stays open and is the graduation pass's one live diagnostic; `/pending 635`
closed; `/pending 637` filed.

### P05 — Checker: annotations (~9 rules) *(done 2026-09-23, v1.153.0)*
**Goal.** Annotation containment, alternate descriptions, tab order, links, media clips, TrapNet, PrinterMark.
**Exit criteria.** As P03.

**(phase-open, 2026-09-23, v1.148.1)** The family is **ten unbuilt** rules, read off veraPDF 1.30.2's own profile
(`PDFUA-1.xml` in the CLI jar): `7.18.1 t1` (an annotation that is not Widget, PrinterMark or Link is nested in an
`Annot` tag), `t2` (it has `/Contents`, or its enclosing element has `/Alt`), `t3` (a field carries `/TU`, or every
widget of it has an enclosing `/Alt`); `7.18.2 t1` (a TrapNet annotation is not permitted); `7.18.3 t1` (a page
carrying annotations declares `/Tabs` `S`); `7.18.5 t1` (a Link annotation is nested in a `Link` tag), `t2` (it has
`/Contents`); `7.18.6.2 t1` (a media clip has `/CT`), `t2` (its `/Alt` is well formed); `7.18.8 t1` (a PrinterMark is
not in the structure tree). **The heading's ~9 is wrong in both directions**: 7.18 holds twelve rules in the profile,
and two of them — `7.18.4 t1` and `t2` — already shipped in P03.S06. Ten rules land the checker at **80 of 106**.

- **Six of the ten share one exemption clause**, `isOutsideCropBox == true || (F & 2) == 2`, and it gets ONE door
  (ADR-009). Read from the parser's source rather than from the profile's wording: `isOutsideCropBox` is
  **disjointness against the INHERITED `/CropBox`, clipped to the `/MediaBox` and falling back to it entirely when
  absent** (`PDAnnotation.java:285-294`, `PDPage.getCropBox():112-119`), compared with `>=`/`<=` so an annotation
  touching the box edge counts as outside — and it answers **null**, not `true`, when either rectangle is missing or
  shorter than four numbers. Null is not the exemption, because the profile tests `== true`. Measured on 1.30.2 for
  the absent, short, reversed, inherited and oversized cases before any rule reads it.
- **The annotation population already has THREE enumerations in the package** — `rules_content.go:125` (7.18.4 t1),
  `rules_language.go:565` (7.2 t24/t25) and `content.go:199` (appearance streams) — and P05 adds up to six more
  subjects over the same array. That is ADR-009's exposure at its widest anywhere in `internal/uacheck`: S01's door
  absorbs the first two, and the appearance walk either joins them or is declared a different question **at its
  site**.
- **`structParentStandardType` for an annotation is the ParentTree element's STANDARD type** (`GFPDAnnot.java:186-202`)
  — the same association 7.2 t24 already builds joined to the same `standardType` door P03.S01 owns. Three of the ten
  are that join, not new reading. `7.18.8 t1` is the exception and reads `structParentType`, the element's raw `/S`.
- **An annotation's `Alt` is the ENCLOSING STRUCTURE ELEMENT's `/Alt`, never the annotation's own key**
  (`GFPDAnnot.java:289-304`), and only when it is a string. `7.18.1 t2` and `t3` both rest on it; reading `/Alt` off
  the annotation dictionary would pass documents veraPDF fails.
- **A widget's `TU` is the FIELD's, and the field is either the widget itself (merged) or its `/Parent` — one level,
  never a climb** (`GFPDWidgetAnnot.java:30-36`). So 7.2 t25's population (form fields) and `7.18.1 t3`'s (widget
  annotations) are different objects over one key: the population trap P04.S03 recorded for t24 against t25, again.
- **`hasCorrectAlt` is a SHAPE, not a presence** (`PDMediaClip.java:58-73`): an array of even length, every entry a
  string, and every ODD-indexed entry non-empty. And a media clip is reached through **actions**, not through
  `/Annots`, so `7.18.6.2` is the one pair of the ten whose subject nib has no path to today.
- **`containsAnnotations` is `!getAnnotations().isEmpty()` over veraPDF's own population** (`GFPDPage.java:200-201`),
  so `7.18.3 t1`'s subject depends on exactly which array entries that population keeps. Measured before the rule is
  written: a page whose only annotation veraPDF drops is a pass there and a fail for a naive `len(/Annots) > 0`.
- **The corpus cannot carry this phase.** Of the 48 files under `7.18 Annotations`, `7.18.2` has **one** and it is on
  `corpusUnreadable` (pdfcpu refuses a TrapNet annotation without its `/F`), `7.18.8` has **one**, a fail, and five
  more sit under `7.18.7`, a clause the 1.30.2 profile does not implement at all. Every clause owes a measured pass
  **and** fail fixture of its own — P03's phase trap, binding harder here than in any phase so far.

#### P05.S01 — the annotation population, one door, and the two general rules *(done 2026-09-23, v1.149.0)*
Scope: `annots()` as the single annotation door — per page, with subtype, `/F`, `isOutsideCropBox` over the inherited
`/CropBox`, `/Contents`, and the `/StructParent` → parent-tree element with its raw `/S`, its standard type and its
`/Alt`; `rules_content.go:125` and `rules_language.go:565` re-expressed over it; then `7.18.1 t1` and `t2`. Refs:
ADR-009, law 5, P03.S01's `standardType`, P04.S03's annotation association.
Acceptance:
- One enumeration of annotations in `internal/uacheck`, and a guard asserts the **routing** (`runtime.FuncForPC`, the
  shape P04.S02's review settled on), with any deliberate exemption named at its own site.
- The crop-box predicate matches veraPDF on measured fixtures for: no `/CropBox` (inherit the `/MediaBox`), an
  inherited `/CropBox`, a `/CropBox` larger than the `/MediaBox`, an absent or short `/Rect`, a reversed `/Rect`, and
  an annotation touching the box edge.
- `7.18.1 t1` and `t2` each hold a measured pass and a measured fail fixture, and each gains a `corpusReach` row
  measured over the 297-file set.
- No shipped clause's verdict moves: `7.18.4 t1`, `7.2 t24` and `7.2 t25` re-measured over the corpus before and after
  the door lands, and the before/after figures recorded.

**(grill, 2026-09-23)** Read from veraPDF's own source — `GFPDAnnot`, `GFPDPage`, `GFPDWidgetAnnot`
(validation-model) and `PDAnnotation`, `PDPage` (parser) — then measured on 1.30.2 over **70 purpose-built
fixtures** before a rule was written. What the reading alone would have got wrong:

- **The population is every `/Annots` entry that resolves to a DICTIONARY**, the array itself possibly
  indirect. Measured, each its own fixture: an annotation written **inline** in the array IS a subject; a
  **dangling reference is NOT**; an entry resolving to an array is NOT; a `/Popup` IS; and an annotation with
  **no `/Subtype` at all** IS.
- **An excluded subtype and an exempt annotation are PASSING CHECKS, not absent subjects.** This is the whole
  shape of the first implementation's error: written as "skip it", nib answered `NotApplicable` on 24 of the
  70 fixtures where veraPDF reports **passed** — the *"veraPDF passed, nib not applicable"* gap P04.S04 named,
  which the oracle scores as agreement and which would have silently halved both clauses' corpus reach. The
  exclusions live in the profile's test EXPRESSION; the population is every annotation.
- **`isOutsideCropBox` is disjointness against the inherited `/CropBox` CLIPPED to the `/MediaBox`**, falling
  back to the media box entirely, with `>=`/`<=` so a **touching edge counts as outside** and an overlap of one
  unit does not. It answers **null** — not `true` — with no `/Rect` or a `/Rect` of fewer than four numbers, and
  the profile tests `== true`, so **an annotation with no rectangle is not exempt**. And the rectangle is used
  **as written**: `[600 600 50 50]` inside a `/CropBox [100 100 500 500]` is OUTSIDE to veraPDF and INSIDE to
  any implementation that sorts the corners first. Measured, with its control.
- **The tag is the enclosing element's STANDARD type** (a private type role-mapped to `/Annot` passes), and the
  `Alt` of 7.18.1 t2 is **that element's**, never the annotation's own key — an `/Alt` on the annotation
  dictionary fails, and both `/Contents` and `/Alt` must be non-empty strings, indirect accepted.

**And the grill found two live false FAILs in a SHIPPED clause, which is why the door lands in this slice
rather than beside it.** `7.18.4 t1` shipped in P03.S06 carrying **neither half of its own exemption** and
refusing a widget written inline in `/Annots` for an `OBJR` the clause does not ask for — the very requirement
that rule's own doc comment says P06.S07 and P07.S03 already removed, surviving in one arm. Four documents
veraPDF **passes** and nib **failed**: a hidden widget outside a Form tag, a widget wholly off the crop box, a
hidden widget under a `P` tag, and an inline widget under a Form tag. **Invisible to the 297-file corpus** —
no corpus file holds a hidden or off-page widget outside a Form tag — and invisible to a rule-by-rule review,
because the defect is a clause of the profile that was never transcribed.

Tasks:
- T01 — `annots()` as the ONE annotation door (`annots.go`): the population above, the page's clipped
  crop box carried on each subject, `/F`'s hidden bit, the tri-state `isOutsideCropBox`, the shared
  `annotExempt`, and `annotElement`'s three-way answer (no `/StructParent` / names nothing / nib could not
  finish the tree).
- T02 — `7.18.1 t1` and `t2` over that door, with the exclusion lists taken from the profile's test
  expression and graded as passes.
- T03 — the three shipped enumerations re-expressed over it: `checkWidgetsInFormElements`,
  `scanAnnotsAndFields` and `walkAppearances`. `7.18.4 t1` gains the exemption and loses the inline arm,
  which changes a shipped clause's verdicts and is measured over the corpus before and after.
- T04 — `TestEveryAnnotationReaderRoutesThroughOneDoor`: an AST scan asserting `annots.go` is the only
  file in the package that indexes a page dictionary by `"Annots"`. Routing, not agreement — three readers
  agreeing said nothing about the fourth, and the one that disagreed was the oldest.
- T05 — fixtures: the 40-row measured table, the four-row `7.18.4 t1` regression with its two controls,
  `AddNotes` as the oracle's product door for both clauses' passing half, and two mutations for the
  failing half that nothing nib ships can write.
- T06 — the count claim: 70 → 72 across the six copies that have a reader, and the complement 36 → 34.

**(acceptance ledger, 2026-09-23, v1.149.0)** Every clause split on `and`; nothing `not exercised`.

| # | clause | how it was discharged |
|---|---|---|
| 1 | one enumeration of annotations in `internal/uacheck` | `annots.go` is the only file in the package holding the literal `"Annots"` — three sites before this slice |
| 2 | and a guard asserts the ROUTING | `TestEveryAnnotationReaderRoutesThroughOneDoor`, an AST scan over every string literal, so `ArrayEntry("Annots")`, `Find("Annots")` and a named const are all caught |
| 3 | with any exemption named at its site | none taken: all three readers joined, the appearance walk included |
| 4 | the crop-box predicate matches veraPDF on six named shapes | all six measured rows plus their controls: no `/CropBox` (media box), inherited, larger than the media box, absent `/Rect`, short `/Rect`, reversed `/Rect`, and the touching edge on BOTH corners |
| 5 | `7.18.1 t1` holds a measured pass and fail fixture | 41 rows, every verdict veraPDF 1.30.2's, plus `AddNotes` and one mutation in the oracle |
| 6 | `t2` likewise | same table; the oracle's failing half is a note with its `/Contents` dropped |
| 7 | each gains a `corpusReach` row measured over the 297-file set | **71** each, exact, and the test fails on a drop |
| 8 | no shipped clause's verdict moves | `7.18.4 t1` 19, `7.2 t24` 71, `7.2 t25` 19 — unchanged, asserted as exact equalities; corpus **0 false pass, 0 false fail** over 21,202 pairs |
| 9 | measured before and after the door lands | the figures above were the before-figures and the test compares against them; the oracle is 4,032 pairs strictly agreeing over 57 documents |

**Beyond the acceptance, and the reason the door lands in this slice:** `7.18.4 t1` had **two live false
FAILs** — no exemption clause at all, and an inline widget refused for an `OBJR` the clause does not ask for.
Four documents veraPDF passes, nib failed. Neither was visible to the corpus (reach 19 at 0 false fails
through P03 and P04) or to a rule-by-rule review, because the defect was a clause of the profile that had
never been transcribed. `TestAHiddenOrOffPageWidgetIsNoLongerAFailure` carries all four with two visible
controls.

**Gates.** `go build ./...`, `go test ./internal/uacheck/` (green, 49s), `./internal/pdfops/`,
`./internal/server/`, `.` (root guards, `TestEveryDocCommentNamesItsOwnFunction` included) all green;
`gofmt -l` clean. Corpus **0 false pass, 0 false fail**, 21,202 pairs, 2 unreadable (both pre-existing).
`/redproof`: **17 mutations, one condition each, 16 red and one declared survivor** — a non-dict `/Annots`
entry, which pdfcpu's reader deletes before any rule sees it (measured: a dangling reference and a `null`
literal are both removed, and an entry resolving to an array makes the validator refuse the file). **Tiers 4
and 6 did NOT fire** — the diff touches neither `internal/server`'s session, ceremony, delivery or discovery
paths, nor `internal/p2p`, nor `internal/rendezvous`; it is `internal/uacheck` plus one count copy in
`internal/pdfops/labelua.go` and two documents.

**Live-verified through the real binary**, not the suite: `nib ua` on veraPDF's own
`7.18.1-t02-fail-a.pdf` reports `7.18.1 t1` pass and `t2` fail naming the offending annotation's object
number, which is exactly what veraPDF reports on that file; on nib's own Markdown conversion and on a real
third-party fillable form both clauses report `not applicable` with "the document has no annotations".

#### P05.S02 — the typed annotations: links, TrapNet and PrinterMark *(done 2026-09-23, v1.150.0)*
Scope: `7.18.5 t1` and `t2` (a Link nested in a `Link` tag; a Link with `/Contents`), `7.18.2 t1` (TrapNet refused),
`7.18.8 t1` (a PrinterMark has no structure parent). Refs: S01's door, P03.S01's standard-type resolution.
Acceptance:
- All four hold a measured pass and fail fixture of their own, because the corpus carries a pass file for neither
  `7.18.2` nor `7.18.8`.
- `7.18.8 t1` reads the **raw `/S`** and not the standard type, measured on a fixture where a role map makes the two
  differ; the same fixture measures which one `7.18.5 t1` reads.
- `7.18.2-t01-fail-a.pdf` either stays on `corpusUnreadable` with its reason restated against this clause, or moves
  off it with the reason retired — stated either way, never left implicit.
- Four `corpusReach` rows, measured.

**(grill, 2026-09-23)** Read from veraPDF's own source (`GFPDAnnot`, `GFPDLinkAnnot`, `GFPDTrapNetAnnot`,
`GFPDPrinterMarkAnnot`, and the profile in the 1.30.2 jar), then measured on **seventeen fixtures** before a
rule was written. Three of the four are one predicate over S01's door; the readings that reading alone would
have got wrong:

- **`7.18.8 t1` reads the element's RAW `/S`, not its standard type** (`GFPDAnnot.getstructParentType` takes
  `getNameKeyStringValue(ASAtom.S)` and stops). It is the ONLY clause in this family that does not go through
  `standardType`, so a private type on a role-map loop — which makes every other clause answer CannotCheck —
  is still a definite Fail here, because the raw name is there to read.
- **And `7.18.8 t1`'s own description reads BACKWARDS.** It says a PrinterMark "shall be considered Incidental
  Artifacts", which invites tagging one `/Artifact`; the test is `structParentType == null`, and measured on
  1.30.2 a printer's mark whose `/StructParent` names an `/Artifact` element **FAILS**. An incidental artifact
  is content *no element describes*, never content described as an artifact. A rule written from the prose
  would pass the document veraPDF fails.
- **A `/StructParent` whose parent-tree row is not an element reads as null and PASSES** — measured with a row
  holding an array. So the clause is not "carries no `/StructParent`"; it is "nothing in the tree types it".
- **`7.18.5 t2` reads the ANNOTATION's `/Contents` and has no fallback**, where `7.18.1 t2` falls back to the
  enclosing element's `/Alt`. The decisive fixture is one document: a link with no `/Contents` whose Link
  element carries `/Alt` **passes 7.18.1 t2 and fails 7.18.5 t2**. Two rules over one annotation, and only a
  description in the right place satisfies both.
- **`7.18.2 t1`'s whole test is the shared exemption**: a TrapNet is permitted only where it is hidden or off
  the crop box. Measured all three ways. A visible TrapNet also fails `7.18.1 t1`, because TrapNet is not on
  that clause's exclusion list — two clauses firing on one annotation is correct here, not a double report.

**Nothing nib writes carries any of these three subtypes** — measured, including the Markdown conversion,
which emits **no `/Annots` at all**. So no product door reaches any half of these four clauses, and the oracle's
six new documents are mutations (`addAnnotation`), which the corpus cannot make up for: it holds **one** TrapNet
file and pdfcpu refuses it, and **one** PrinterMark file, a fail.

Tasks:
- T01 — `annotsOfSubtype` over S01's door: the population of a typed clause is the annotations of one
  `/Subtype`, which is how veraPDF picks the model subclass the profile names as each rule's object.
- T02 — `7.18.5 t1` and `t2`, `7.18.2 t1`, `7.18.8 t1`, each with the shared exemption and the same verdict
  precedence S01 established (a definite Fail on a read annotation beats a refusal from a short population).
- T03 — `7.18.8 t1` reads the raw `/S`, with a role-map loop fixture proving it does not consult the map and
  its 7.18.1 t1 control proving the same document does make another clause refuse.
- T04 — fixtures: the seventeen-row measured table, the decisive `7.18.1 t2` / `7.18.5 t2` pair on one
  document, and `addAnnotation` — a mutation that injects an annotation, a structure element and a
  parent-tree row at the next free key.
- T05 — the count claim: 72 → 76 across the six copies with a reader, and the complement 34 → 30.
- T06 — `corpusReach`: 31, 31, 1 and **0**, the zero recorded rather than omitted, with the reason.

**(acceptance ledger, 2026-09-23, v1.150.0)** Every clause split on `and`; nothing `not exercised`.

| # | clause | how it was discharged |
|---|---|---|
| 1 | all four hold a measured pass fixture of their own | the 23-row table, every verdict veraPDF 1.30.2's, plus six oracle documents |
| 2 | and a fail fixture | same table; the oracle reaches both halves of all four (law 5 green over 60 documents) |
| 3 | because the corpus carries a pass file for neither 7.18.2 nor 7.18.8 | measured: `7.18.2` reach is **0** — its one corpus file is unreadable to pdfcpu — and `7.18.8` reach is **1**, a fail |
| 4 | `7.18.8 t1` reads the raw `/S` and not the standard type | `TestAPrinterMarksTagIsReadRawAndNotThroughTheRoleMap`: a role-map loop leaves it a definite Fail, and a `/Text` annotation in the SAME looped element makes 7.18.1 t1 refuse — the control that proves the difference is the role map and not the fixture |
| 5 | measured on a fixture where a role map makes the two differ | `/MyMark → /Artifact` fails, and so does the loop; both rows measured |
| 6 | the same fixture measures which one 7.18.5 t1 reads | it reads the STANDARD type: `/MyLink → /Link` passes, and a looping `/MyLink` is CannotCheck |
| 7 | `7.18.2-t01-fail-a.pdf` stays on `corpusUnreadable` with its reason restated | stated, and the reason is `/F` and not `/AP` — read from pdfcpu's `validateAnnotationDictTrapNet`, which requires neither `/AP` nor anything else but `/F` |
| 8 | four `corpusReach` rows, measured | 31, 31, 1 and **0**, the zero recorded with its reason rather than omitted |

**Gates.** `suiterun` green (fingerprint in the close notes); `go test ./internal/uacheck/` 45s; corpus **0 false
pass, 0 false fail** over 22,382 pairs; `len(Clauses())` = **76**, run rather than counted. `/redproof`: **8
mutations, one condition each, all red** after the review's fixes — five of them were survivors the review
found. **Tiers 4 and 6 did NOT fire**: the diff is `internal/uacheck` plus one count copy in
`internal/pdfops/labelua.go` and two documents.

**Live-verified through the real binary**: `nib ua` on veraPDF's own `7.18.5-t01-fail-a.pdf` reports
`7.18.5 t1` fail — naming the link's object number — and `7.18.5 t2` pass, which is exactly veraPDF's pair of
verdicts on that file.

**What the review found, and it is the reason this slice is not four one-line rules.** Six coverage holes and
one **live false pass**: an element whose `/S` is an EMPTY name. `d.name` cannot tell `/S /` from no `/S` at
all, veraPDF's `getNameKeyStringValue` yields `""` there and the profile tests `structParentType == null`, so
veraPDF FAILS a printer's mark in such an element and nib passed it. Measured: pdfcpu accepts the file. Fixed
by a `nameOf` door that answers presence separately from value — which the package's own ADR-009 guard then
required be moved into `document.go`, where every typed reader lives.

#### P05.S03 — the interactive surface: a widget's alternative description, and the page's tab order *(done 2026-09-23, v1.151.0)*
Scope: `7.18.1 t3` (the field's `/TU`, else every one of its widgets has an enclosing `/Alt`) and `7.18.3 t1` (a page
carrying annotations declares `/Tabs` `S`). Refs: S01's door, 7.2 t25's field population.
Acceptance:
- The widget → field `/TU` read is one level (`/Parent`), never a climb, measured against veraPDF on a fixture whose
  `/TU` sits on a grandparent field.
- `7.18.3 t1`'s subject population is measured against veraPDF's `containsAnnotations` on pages whose only annotation
  is hidden, outside the crop box, a Popup, or an unresolvable reference.
- Both clauses hold a measured pass and fail fixture and a `corpusReach` row.

**(grill, 2026-09-23)** Read from `GFPDWidgetAnnot.getTU`, `PDFormField.isField`, `GFPDPage.getcontainsAnnotations`
and `PDPage.getTabs`, then measured on **thirteen fixtures** before a rule was written, and on **seven more** during
the review. What reading alone would have got wrong:

- **A widget's `/TU` is the FIELD's, and the widget is the field only when it carries `/T`** — `isField` is
  `knownKey(ASAtom.T)`, the key's presence whatever it holds (`PDFormField.java:186-188`). The lookup is **one
  level through `/Parent`, never a climb**: measured, a `/TU` on the GRANDPARENT field is not found, and a kid
  widget's **own** `/TU` is **ignored**. Reading the annotation's `/TU` unconditionally passes two documents
  veraPDF fails.
- **`7.18.3 t1` carries NO exemption**, the only clause over this door that does not: a hidden annotation and one
  wholly off the crop box both still oblige the page to declare its tab order (measured, both). Its subject is the
  PAGE, so a page with no annotations is a **passing check** rather than no subject, and the clause answers
  NotApplicable for no document that has a page.
- **`/Tabs` is inherited by neither side.** `PDPage.getTabs` is a plain `getKey` with no page-tree walk, which
  matches ISO 32000-1 Table 30; nib reads the page's own dictionary. An INDIRECT `/Tabs` naming `/S` passes on
  both (measured) — the value is dereferenced, the key is not inherited.

Tasks:
- T01 — `fieldTU`: the field is the widget when `/T` is PRESENT, else its `/Parent`, one level.
- T02 — `7.18.1 t3` over the widget population with the shared exemption and the enclosing element's `/Alt`.
- T03 — `7.18.3 t1` over the door's per-page view, reading the page's own `/Tabs`.
- T04 — fixtures: the thirteen-row table, the seven `/T` shapes, and the tab-order population rows.
- T05 — the count claim: 76 → 78, and the complement 30 → 28.
- T06 — `corpusReach`: 19 and 295.

**(acceptance ledger, 2026-09-23, v1.151.0)** Every clause split on `and`; nothing `not exercised`.

| # | clause | how it was discharged |
|---|---|---|
| 1 | the widget → field `/TU` read is one level, never a climb | the grandparent row fails, measured; probed red by making it climb |
| 2 | measured against veraPDF on a fixture whose `/TU` sits on a grandparent field | `W5`, and its control `W4` one level up passes |
| 3 | `7.18.3 t1`'s subject population is measured against `containsAnnotations` | hidden, off-crop-box, Popup, empty `/Annots`, no `/Annots`, and a dangling entry — six shapes, all measured |
| 4 | on a page whose only annotation is hidden / outside the crop box / a Popup / unresolvable | all four measured; the unresolvable one is a **declared divergence**, not parity (below) |
| 5 | both clauses hold a measured pass and fail fixture | thirteen rows plus two oracle documents each, **both passing halves from PRODUCT doors** (`AuthorForm` writes `/TU`, `setStructureTabOrder` writes `/Tabs /S`) and one failing half too (an unlabelled field writes no `/TU`) |
| 6 | and a `corpusReach` row | `7.18.1 t3` at **19**, `7.18.3 t1` at **295** — the highest row in the table, because its subject is a page |

**Gates.** `suiterun` green (fingerprint in the close notes); corpus **0 false pass, 0 false fail** over 22,972
pairs; `len(Clauses())` = **78**. `/redproof`: **6 mutations, one condition each, all red.** Tiers 4 and 6 did NOT
fire. **Live-verified through the real binary on real third-party documents in BOTH directions**:
`7.18.4-t01-pass-a.pdf` passes both clauses in nib and veraPDF alike, and `7.18.1-t03-fail-a.pdf` fails
`7.18.1 t3` in both.

**The review found a live FALSE PASS, and it was in the identity test rather than in either rule.** nib decided
"is this widget the field" by reading `/T` as a STRING where veraPDF asks only whether the KEY is present. A `/T`
naming a **free object** is a document pdfcpu accepts, veraPDF **fails** and nib **passed** — because a dangling
reference is not a string, so nib read the parent field's `/TU` instead of the widget's own absent one. Fixed to a
presence test.

**Two further things the slice got wrong and the measurement caught, both worth recording.** First, my own
six-shape sweep of `/T` had **missed the dangling case** — it covered name, number, empty, indirect-resolving and
null, which is why the reviewer's reading found what my measuring did not. Second, I then reported name- and
number-typed `/T` as *"unreachable, pdfcpu refuses them"* — and that was **wrong**: those two fixtures had failed
to open for a **missing `/DA`**, not because of `/T`'s type. The new test asserts the refusal explicitly so that a
pdfcpu bump which starts accepting them is caught here, and its first draft was itself wrong for a third reason —
an inline widget dictionary left `/Kids [30 0 R]` dangling, so pdfcpu never validated the field at all.

#### P05.S04 — media clips, a population reached through actions *(done 2026-09-23, v1.152.0)*
Scope: `7.18.6.2 t1` (`/CT` present) and `t2` (`hasCorrectAlt`): the media-clip population — screen annotations,
rendition actions and whatever else veraPDF's model reaches — and the `/Alt` array's shape. Refs: `PDMediaClip.java`,
S01's door for the annotation half of the path.
Acceptance:
- The population is stated from veraPDF's source (which actions and which keys reach a media-clip dictionary) and
  measured on a fixture for each path, including a clip reached from a document-level or additional action rather
  than from an annotation — or that path is a declared, measured CannotCheck.
- `hasCorrectAlt` matches veraPDF on: odd length, a non-string entry, an empty even-indexed entry, an empty
  odd-indexed entry, an absent `/Alt` and a non-array `/Alt`.
- Both clauses hold a measured pass and fail fixture and a `corpusReach` row.
- `len(Clauses())` is **80**, run rather than counted, and the README, the parity doc and the complement figure move
  with it through their existing readers.

**(grill, 2026-09-23)** Read from `GFPDRenditionAction`, `PDAction`, `GFPDAnnot`, `GFPDOutline`, `GFPDDocument`,
`GFPDPage`, `GFPDAdditionalActions` and `PDAbstractAdditionalActions`, then measured on **33 documents** —
eleven `/Alt` and `/CT` shapes, the corpus's own five files, and one per holder path. The population is the whole
slice; both predicates are three lines each.

- **A clip sits at `<action>/R/C` for a `/S /Rendition` action**, and an identical `/R`→`/C` under a `/GoTo` or a
  `/Movie` action is graded by neither reader (measured, both).
- **`hasCorrectAlt` is a SHAPE**: an array of EVEN length, every entry a string, every ODD-indexed entry
  non-empty. An empty LANGUAGE passes — it is Table 274's default entry — and an **empty array passes too**,
  which is the row that shows the predicate tests shape and not "there is a description".
- **Seven holders reach an action**, plus the action's own `/Next` chain: an annotation's `/A` and `/AA`, an
  outline item's `/A`, the catalog's `/OpenAction` and `/AA`, a page's `/AA`, and a form field's `/AA`.
- **The `/AA` trigger names are a FIXED LIST PER HOLDER**, not every entry of the dictionary:
  ten for an annotation, two for a page, five for the catalog, four for a field. Walking every entry grades a
  clip veraPDF ignores.

Tasks:
- T01 — `mediaClips()`: the seven holders, the per-holder trigger lists, the `/Next` chain, and a reason recorded
  wherever a bound truncates.
- T02 — `7.18.6.2 t1` and `t2` over that population.
- T03 — fixtures: the eleven shapes, the eight holder paths, the two non-Rendition controls, the outline's
  sibling and child edges, and the truncation refusal.
- T04 — the count claim: 78 → 80, and the complement 28 → 26.
- T05 — `corpusReach`: 5 and 5, the phase's last rows.

**(acceptance ledger, 2026-09-23, v1.152.0)** Every clause split on `and`; nothing `not exercised`.

| # | clause | how it was discharged |
|---|---|---|
| 1 | the population is stated from veraPDF's source | seven holders plus `/Next`, each cited to the class and line that links it |
| 2 | and measured on a fixture for each path | eight rows, each with a clip missing `/CT` so a dropped path shows up as a Pass |
| 3 | including a clip reached from a document-level or additional action | the catalog's `/OpenAction` and `/AA`, a page's `/AA`, a field's `/AA /K`, and an outline item's `/A` |
| 4 | `hasCorrectAlt` matches veraPDF on odd length, a non-string entry, an empty even entry, an empty odd entry, an absent `/Alt` and a non-array `/Alt` | all six measured; three of them are pdfcpu refusals, asserted by the refusal they actually print |
| 5 | both clauses hold a measured pass and fail fixture | the corpus's five files cover both halves of both — **the only clause family in this phase with real corpus evidence on all four halves** |
| 6 | and a `corpusReach` row | 5 and 5 |

**Gates.** `suiterun` green (fingerprint in the close notes); corpus **0 false pass, 0 false fail** over 23,562
pairs; `len(Clauses())` = **80**, which completes the phase's rule set. `/redproof`: **14 mutations, 13 red and
one declared survivor** — reading `/CT` as presence-of-any-type, which pdfcpu's refusal of a non-string `/CT`
makes unreachable, recorded at the line with a row asserting that refusal. Tiers 4 and 6 did NOT fire.

**The review found FOUR live divergences, all in the population, and one of them is this session's lesson
repeating itself.** Two false PASSES — a form field's `/AA` under its own triggers `{K, F, V, C}`, and an
action's `/Next` chain in both its dictionary and array forms — one false FAIL (a `/A` read on the catalog, which
has no such key in veraPDF's model), and one **silent truncation**: the outline walk stopped at 65 items in a
chain without recording a reason, so a document with 70 bookmarks — an ordinary table of contents — would have
had its last item's clip unread and both clauses would have answered Pass over it.

**The form-field path is the cautionary one.** I had dropped it earlier in the slice on a measurement showing
veraPDF evaluated nothing — and that fixture used `/U`, a trigger in the ANNOTATION list and absent from the
field's. The null result was a fact about my trigger name, not about veraPDF's traversal. Re-measured with `/K`:
veraPDF fails, nib passed. **The same error as P05.S03's `/T` sweep, one slice later**, which is why
`mediaclips.go` now writes out all four lists at the site and says why.

**Phase close (2026-09-23, v1.153.0).** Acceptance ledger over the exit criteria — *"As P03"* — and
every phase-open amendment, each clause split on `and`; nothing `not exercised`.

| # | clause | how it was discharged |
|---|---|---|
| 1 | *"Every rule in the family passes `veracorpus_test.go`"* | **met**, run rather than asserted: 297 files, **23,562 (file, clause) pairs, 2 unreadable, 0 false pass, 0 false fail**. All ten of the phase's clauses carry an exact `corpusReach` row, and `veracorpus_test.go:187` fails a clause that has none |
| 2 | *"rules without corpus files agree with veraPDF on fixtures of their own"* | **met**: `7.18.2 t1` has corpus reach **ZERO** — the corpus's one TrapNet file is on `corpusUnreadable`, pdfcpu refusing it — and `7.18.8 t1` has reach 1. Both are carried by their own measured fixtures, and the in-repo oracle runs veraPDF live: **5,600 of 5,600 (document, clause) pairs agree strictly over 70 documents** |
| 3 | *"The family is **ten unbuilt** rules, not the sketch's ~9"* | **met**: `7.18.4 t1`/`t2` shipped in P03.S06, and the ten built here are 7.18.1 t1/t2/t3, 7.18.2 t1, 7.18.3 t1, 7.18.5 t1/t2, 7.18.6.2 t1/t2, 7.18.8 t1 |
| 4 | *"the phase lands the checker at **80** of 106"* | **met**, run not counted: `len(Clauses())` = 80, complement 26, and `TestPassingEveryClauseNibChecksIsNotConformanceAndTheDocsSaySo` derives both from it and requires them in the README, the parity doc and three prose sites — six files go red on an 81st registration |
| 5 | *"S01's door absorbs the first two enumerations"* | **met**: `rules_content.go` and `rules_language.go` route through `annots()`, and `TestEveryAnnotationReaderRoutesThroughOneDoor` asserts `"Annots"` appears at exactly one non-test site |
| 6 | *"and the appearance walk either joins them or is declared a different question **at its site**"* | **met**: `content.go`'s walk routes through the door too, and the door's own header states which entries veraPDF keeps and which nib's reader can never see |
| 7 | *"Every clause owes a measured pass **and** fail fixture of its own"* — P03's trap, binding harder here | **met by enumeration, and that is the honest wording**: all ten hold both halves, S04's four halves resting on real corpus evidence. **There is no standing reader that would catch an eleventh clause landing without them** — the gate is `corpusReach`, which asks for a row and not for a pass fixture. Filed as the phase's own declared gap rather than claimed as covered |

**Gates, enumerated explicitly because they are a separate list from the criteria.**

- **Tiers 0–3** — `suiterun` **OK** on fingerprint `f6570b8c4a68` and again after the phase-close
  fixes: `go build ./...` and `go test ./...` exit 0, jsdom **454/454**, tier-3 `uirepro` **160/160,
  0 skipped**. Tier 3 is fully green; the three `/pending 474` baseline reds are gone.
- **Tier 4 (`pairrepro.sh`) and tier 6 (`ceremonyrepro.sh`): DID NOT FIRE, and here is the evidence
  rather than the assertion.** `git diff --name-only aee7e76..HEAD` over the phase's whole life is
  `internal/uacheck/*`, `internal/pdfops/labelua.go` (a comment carrying the rule count), `README.md`,
  `docs/accessibility-parity.md`, `VERSION` and this plan. No `internal/server` session, ceremony,
  delivery or discovery path; no `internal/p2p`; no `internal/rendezvous`. The trigger is recorded
  not-fired because a gate nobody records is one nobody can tell was evaluated.
- **`inventorycheck --phase=P05`** — ok, 4 shipped slices each with a section, **279 readers all
  resolve, 0 declared gaps**, re-run after the fixes landed.

**The full-repo review is the phase close's own, and it found the thing a phase close is for.** Eight
parallel reviewers over every package, three SME packs (`go` core+service+wire, `crypto` core,
`verification`); hand-off at
`~/.claude/projects/-home-dan-repos-nib/memory/code-reviews/v1.152.0-2026-09-23.md`.

Inside the phase it found **one predicate with two implementations that disagreed**: the
`/StructParent` → element hop. `annots.go` kept a `found` check and answered DEFINITE for a row that
is present and is not a dictionary; `rules_language.go` had no `found` check, so on a tree that is
BOTH truncated and holds such a row, 7.2 t24/t25/t29 answered `CannotCheck` over the very row
7.18.1 t1 called a definite `Fail`. **`annots.go` already carried a comment calling that exact
collapse found-and-fixed** — fixed in the door P05.S01 wrote and not in the door P04.S03 wrote beside
it, which is ADR-009's shape and is invisible to any guard that compares answers rather than counting
sites. The door now takes a holder, the guard counts `"StructParent"` sites as well as `"Annots"`
ones, and both are red-proved.

Two silent truncations went with it, both the same class as the 65-bookmark outline truncation S04's
own review found: the AcroForm walk discarded `DereferenceArray`'s error (so `7.2 t25` said *"the
document has no form fields"*, and since S04 the **media-clip population is built on that same walk**,
so a dropped field path would have left 7.18.6.2 answering Pass over a clip nobody looked at), and
`parentTree` skipped a present-but-unreadable `/Nums` or `/Kids` without setting `ptErr` (so
`elementForStructParent` took its definite branch and reported *"names no element"* over a tree nib
never read). Both are **measured unreachable through a file** — pdfcpu v0.13.0 refuses a non-array in
either position before any rule runs — so both readers are built in memory through `openMutated`, on
the footing that helper already declares, and both are red-proved.

**Everything else the review found is outside this phase and is filed, not fixed** — `/pending
639-653`, fifteen items including three criticals, one of which (`/pending 615`) was already open.
The two new ones are `nib watch --do <transform>` writing back through a symlink-following door while
its two siblings deliberately do not, and `Encrypt` denying **every** permission bit including
extraction for accessibility, measured at `/P = -3901`. Neither is reachable from this phase's code.

**Pending sweep against the closure:** the `Phase` section holds exactly two items, `/pending 457`
(`PLAN-text-reflow.md`'s P05 — a different plan) and `/pending 493` (this plan's **P07**). **No item
was gated on the coordinate that just closed**, so nothing was falsified by it and nothing needed
re-filing. Recorded because a sweep that finds nothing and a sweep that never ran look identical.

**Graduation pass** — `<project-memory>/instruments/ua-coverage.md`, run after the fix pass. **44 data
rows across S01–S04, every one class 1 with a named standing reader, so all 44 disposition
`keep-live`; the actionable subset is ZERO** (no row tagged `diagnostic, no standing reader`, no hot
row). That is what a checker's inventory looks like — every observable here IS a shipped assertion —
and it is recorded with both counts, plus what the pass could not see.

### P06 — Checker: file-level rules (~11 rules) *(done 2026-09-23, v1.158.1)*
**Goal.** Identification prefix and properties, header, Suspects, embedded-file keys, XFA, encryption P,
reference and Form XObjects, Formula.
**Exit criteria.** As P03.

**(phase-open, 2026-09-23, v1.153.0)** The family is **eleven** rules and the sketch's "~11" is right for
once — counted the way P05's open counted, from veraPDF 1.30.2's own `PDFUA-1.xml` inside
`~/verapdf/bin/cli-1.30.2.jar` (106 rules parsed, 80 diffed against `Clauses()`, 26 unbuilt). P06 takes
eleven of the 26 and lands the checker at **91**; the other fifteen are P07's, which is **14 font rules
plus `7.1 t12`**, not the sketch's ~12.

The eleven: `5 t3`/`t4`/`t5` (the `pdfuaid` prefix on `part`, `amd`, `corr`), `6.1 t1` (the file header),
`7.1 t4` (`Suspects != true`), `7.7 t1` (Formula carries `/Alt` or `/ActualText`), `7.11 t1` (an embedded
file's `F` and `UF`), `7.15 t1` (no dynamic XFA), `7.16 t1` (encryption `/P` bit 10), `7.20 t1` (no
reference XObjects), `7.20 t2` (a Form XObject's content has one semantic parent).

**`7.1 t12` is NOT in this phase.** P03's close registered it as *declared, not registered* — veraPDF
1.30.2 cannot fail it for an element reached through the tree — and nothing about this phase changes that.
It is carried to P07 with its declaration intact, not silently absorbed here.

**TWO couplings the sketch could not see, both measured at this open.**

- **`7.7 t1` IS the counterexample the docs cite, and building it RETIRES that counterexample.** The
  README, `door.go`, `rules_catalog.go` and `docs/accessibility-parity.md` all say *"a paragraph tagged as
  a formula with no alternate text passes all of Nib's checks and fails veraPDF"*, and
  `TestADocumentCanPassEveryClauseNibChecksAndStillFailVeraPDF` builds exactly that document and asserts
  it. **That test is designed to go red here** — its own doc says *"If a future rule makes nib catch it,
  this test says the docs need a new example"* — and the slice that lands `7.7 t1` therefore owes a NEW
  counterexample and four prose sites, not a deleted test. ADR-031 law 1 rests on there being one: a
  checker that cannot name a document it passes and veraPDF fails is a checker claiming conformance.
  This is why `7.7 t1` gets a slice of its own rather than riding along with a key read.
- **`7.16 t1` will FAIL nib's own `Encrypt` output**, measured at this open and filed the same day as
  `/pending 640`: `Encrypt` never sets `conf.Permissions`, so the written `/P` is `-3901` = `0xF0C3`, and
  `0xF0C3 & 512 == 0` — bit 10, *"extract text and graphics in support of accessibility"*, is denied. The
  checker rule and the writer defect are two halves of one fact, so S03 builds the rule and the phase says
  plainly that nib's own protected output does not pass it. **Which permissions `Encrypt` should grant is
  Dan's call** (`/pending 640`), so the slice does not silently pick one.

**`/plan-review` did NOT fire.** The trigger is a security-, migration- or egress-heavy phase; this one
adds eleven read-only clauses to a checker, writes no bytes, moves no format, and sends nothing. `7.15 t1`
(XFA) and `7.16 t1` (encryption) touch security-flavoured *subjects* without changing any security
behaviour. Recorded because a trigger nobody records is one nobody can tell was evaluated.

**Phase close (2026-09-23, v1.158.1).** Acceptance ledger over the exit criteria ("As P03") and every phase-open
amendment, split on `and`:
- *"Every rule in the family passes `veracorpus_test.go`"* — **met**: 297 files, 26,806 scored pairs, 0 false pass,
  0 false fail; all eleven of the phase's clauses registered, each with an exact `corpusReach` row.
- *"rules without corpus files agree with veraPDF on fixtures of their own"* — **met**: `6.1 t1` has no corpus file
  and `7.15 t1` no passing one; the live oracle (**7,735 of 7,735 pairs over 85 documents**) reaches BOTH veraPDF
  states for every P06 clause and declares none unreachable (`notYetReachable` holds no P06 row).
- *"eleven rules … lands the checker at 91"* — **met**: 91 of 106, guarded by the registry-count test.
- *"`7.1 t12` is NOT in this phase … carried to P07 with its declaration intact"* — **met**: still unregistered,
  still declared.
- *"`7.7 t1` … RETIRES that counterexample … owes a NEW counterexample and the prose sites"* — **met** at P06.S04.
- *"`7.16 t1` will FAIL nib's own `Encrypt` output … the phase says plainly"* — **met, and corrected**: the phase-close
  review found no product door REPORTS it (`Encrypt` sets a user password, and the checker reads without one), so the
  rule's comment and test now say the failure is the writer's predicate. Which permissions to grant is /pending 640,
  Dan's.

**The full-repo review** (eight reviewers, `code-reviews/v1.158.0-p06-phase-close-2026-09-23.md`) found 71 findings,
six critical — **none of the criticals in this phase's package**. Inside it, R1's seven were all fixed, and two were
live disagreements with veraPDF in rules this phase BUILT, invisible to both oracles because the corpus's one `7.15`
document fails and no document varied the `part` value:
- `7.15 t1` read every packet at any depth and trimmed; veraPDF reads the stream after the first STRING `config`,
  walks `xdp:xdp?/config/acrobat/acrobat7/dynamicRender` taking the first child of each name, and takes the first
  child NODE's raw value. The port found more than the review listed — veraPDF merges text across comments but not
  across CDATA — and found **P06.S02's own tests had asserted behaviour nobody measured**: their fixtures named no
  `config` entry at all. Three re-review rounds then found eleven shapes Go's decoder accepts and veraPDF's parser
  refuses (false FAILS), each now refused and pinned; the rest of XML well-formedness is declared and filed
  (/pending 673) rather than assumed.
- `5 t2` read `part` trimmed and first-run and compared strings; veraPDF reads the whole raw text as an integer (`01`
  and `+1` pass, ` 1 ` fails), and a qualified property's value is its `rdf:value`, element or attribute form.
- The checker's pdfcpu configuration is now ONE door that pins every read-shaping field, used by all three reads;
  `optimize` off was measured to move `7.20 t2`'s answer (and to give veraPDF's EXACT answer where nib now refuses —
  /pending 672).
Outside the phase: 64 findings filed as /pending 660-671 (five criticals individually: a shared consent slot that
signs the wrong ceremony, a decoy field hiding appended content, a transport certificate expiring 15 minutes into an
arm, `pagenum` overwriting an input, pdfops' unbudgeted form walks) and /pending 580 re-confirmed.

**Red-proof of the fix pass**: 18 targeted mutations, 18 red; 5 blind, 4 red on first run and the survivor (the LAST
`config` entry read instead of the first) closed by a measured pin. **Graduation**: 66 P06 inventory rows, 60
`keep-live` mechanically, 6 actionable all `keep-live`, 0 gated, 0 deleted (`instruments/ua-coverage.md`).
**Required-run gates**: tiers 4 and 6 NOT FIRED — no slice nor the close touched a session, ceremony, delivery,
discovery, p2p or rendezvous path (the diff is `internal/uacheck`, one `pdfops` comment, docs). **Pending sweep
against the closure**: 654/655/656 stay open (the phase never touched them), 657 is P07's, 658/659 new; nothing closed.

#### P06.S01 — the identification's prefixes, and the file header *(done 2026-09-23, v1.154.0)*
Scope: `5 t3`, `5 t4`, `5 t5` — each property of the PDF/UA identification schema carries namespace prefix
`pdfuaid`, with `null` passing — and `6.1 t1`, the header being `%PDF-1.n` for a single digit 0–7 followed
by a single EOL marker. Refs: the package already reads XMP for `5 t1`/`t2` (`xmpFacts`, `readXMP`).
Acceptance:
- The three prefix clauses read the PREFIX, not the value: a `part` of `1` under a namespace bound to a
  different prefix fails, and an ABSENT property passes (the profile is `partPrefix == null || …`).
- `6.1 t1` is nib's first reader of the file's raw leading bytes; it states where those bytes come from and
  what it does when the document was rebuilt in memory rather than read from disk.
- The EOL half is measured, not assumed: veraPDF's test is a regex over `header`, so what that string
  contains — and whether a `%PDF-1.7\r\n` passes — is read off the parser's source before the rule is
  written.
- Both clauses hold a measured pass and fail fixture and a `corpusReach` row; `len(Clauses())` is 84.

**(grill, 2026-09-23 — *(in progress)*)** Everything below was MEASURED against veraPDF 1.30.2, not read
off a description. The four profile tests are `partPrefix == null || partPrefix == "pdfuaid"` (likewise
`amd`, `corr`) and `/^%PDF-1\.[0-7]$/.test(header)`.

**The grill found TWO live defects in the SHIPPED `5 t1`/`5 t2`, and they are this slice's first two
tasks.** The subject gate for the whole family is `containsPDFUAIdentification`, and it is **not**
`part`: measured by three length-preserving mutations of `5-t03-pass-a.pdf`, a packet carrying
`pdfuaid:corr` and no `part` PASSES `5 t1` and FAILS `5 t2`, and so does one carrying an unknown
`pdfuaid:zzzz`, while a packet with the pdfaExtension schema describing the pdfuaid namespace but **no
property in it** FAILS `5 t1` and gives `5 t2`/`t3`/`t4`/`t5` **no subject** (0 passed, 0 failed). So the
identification exists iff the packet carries **at least one property in the pdfuaid namespace**, whatever
its local name. nib's `checkUAIdentification` returns `Fail` on that first document (a live **false
fail**) and `checkUAPartValue` returns `NotApplicable` where veraPDF fails (a live **false pass**). The
corpus cannot see either: it has no such file.

**An absent `amd`/`corr` is a PASSING check, not an absent subject** — P05's measured trap again.
`5-t03-pass-a.pdf` carries neither and veraPDF reports `passedChecks="1"` for both `5 t4` and `5 t5`.

**Go's `encoding/xml` cannot implement these three clauses as `parseXMP` is written.** `Token()` resolves
a prefix to its URI and discards the prefix, and the inversion "which prefix maps to this URI" is
ambiguous on exactly the fixture that matters: `5-t04-fail-a.pdf` binds **both** `pdfuaia` and `pdfuaid`
to `http://www.aiim.org/pdfua/ns/id/`. The reader therefore moves to `RawToken()` with nib's own
namespace scope stack — which also means nib now owns the well-formedness check `Token()` was supplying,
so a mismatched or unclosed tag must still reach `Readable=false` or `7.1 t9`, `5 t1`, `5 t2` and
`7.2 t33` silently stop being `CannotCheck` on a broken packet.

**`6.1 t1`: `header` is the first line from `%PDF-`, not the eight-byte match.** Measured by four
length-preserving header mutations, reading the value back out of veraPDF's own `%1` argument:
`%PDF-1.9` fails and reports `%PDF-1.9`; `%PDF-2.6` fails; replacing the EOL byte with `X` fails and
reports **`%PDF-1.6X%öäüß`** — the rest of the line, so nothing is truncated; and replacing the EOL with a
lone `\r` **PASSES**, so a CR terminates the line. The ISO sentence's "followed by a single EOL marker"
is therefore tested only in the sense that any byte between the digit and the EOL lengthens the string
past the anchored regex: `%PDF-1.7\r\n` passes, `%PDF-1.7 ` fails. **nib matches veraPDF, and the
divergence from the ISO text is declared rather than fixed.**

**Where the bytes come from.** `d.raw` (`document.go:81`), already the second reader of it after
`hasInlineType3Font` (`rules_language.go:652`). A `*Document` built in memory by `openMutated` has no
`raw`, so the clause is `CannotCheck` naming that — never a `Pass` over bytes nib never saw.

**`6.1 t1`'s `corpusReach` is the whole corpus and `5 t3`/`t4`/`t5`'s is small** — the corpus carries
`5-t03-pass-a`/`fail-a`, `5-t04-fail-a`, `5-t05-fail-a`. There is no `6.1` directory, so unlike
`7.18.2 t1`'s zero this clause's reach comes from every file having a header.

Tasks:
- `T01 — 5 t1's subject is any pdfuaid property, not part` (the false fail).
- `T02 — 5 t2 fails an identification with no part` (the false pass).
- `T03 — parseXMP moves to RawToken with nib's own scope stack, keeping well-formedness`.
- `T04 — 5 t3, 5 t4, 5 t5 over one prefix door`.
- `T05 — 6.1 t1 in rules_file.go, reading d.raw`.
- `T06 — oracle documents reaching both halves of all four clauses; corpusReach rows; notTreeRules`.

**(review, 2026-09-23 — three reviewers over the diff, packs `go` + `verification`)** The slice's own
tests were green and its two oracles agreed when the review started; **every finding below is something
neither the corpus nor the oracle could see**, and four were live divergences from veraPDF.

- **`6.1 t1` read the wrong string, in both directions.** `header` is the whole LINE that first mentions
  `%PDF-`, taken from the line's START — measured: veraPDF FAILS `ZZZZ…%PDF-1.6` (junk on the header's own
  line) and PASSES the same junk on a prior line. Slicing at the `%PDF-` occurrence made the profile's `^`
  anchor **unfalsifiable**, so nib passed the file veraPDF fails; and a 1 KiB search window refused a file
  with 2,144 bytes of preamble that pdfcpu opens and veraPDF passes. Seven header shapes now agree exactly.
- **Attribute-form XMP was invisible, a false FAIL across a whole serialisation.** RDF/XML's abbreviated
  syntax writes a simple property as an attribute on `rdf:Description`. Measured: veraPDF passes all five
  clause-5 tests on an attribute-form identification while nib reported `5 t1` Fail and the other four
  NotApplicable. The corpus cannot see it — every corpus identification is element-form. An UNPREFIXED
  attribute is in no namespace and must NOT create a subject; that is its own case.
- **The reader was an attacker-supplied quadratic.** Resolving a prefix walked the open-element stack where
  `Token()` had kept a flat map. Measured through `Check`: 5,000 deep 57 ms, 20,000 deep 667 ms, 80,000 deep
  **11.1 s**. The packet is a flate stream inside a PDF the user opened and nothing bounds its decoded size.
  One binding map popped at each end tag restores O(1), and `maxXMPDepth` refuses past 1,000 as `CannotCheck`
  — measured after: 100/400/900 deep is 81µs/177µs/403µs.
- **Three coverage holes with no divergence behind them, each closed by a test that now goes red**: a
  property's PREFIX and VALUE could be taken from different elements; `5 t4` and `5 t5` were
  **interchangeable** (swapping their properties left everything green but the corpus, which is skipped where
  veraPDF is absent); and the `[0-7]` digit class — the clause's namesake — was asserted nowhere, because
  pdfcpu refuses `%PDF-1.8`/`1.9` before any rule runs.
- **`checkUAPrefix`'s `!declared` branch was INERT** — an absent property and a default-namespace one both
  yield an empty prefix, so the two cases are one comparison, stated once.

**Nineteen red-proof probes across two passes; four survivors, all closed.** The blind pass — an adversary
that never saw the tests — found the one the targeted pass structurally could not: every targeted probe used a
WRONG prefix and none a MISSING one, so the family agreed with itself and nib refused a default-namespace
property veraPDF passes.

**Gates.** Oracle **6,132 of 6,132** over 73 documents; corpus **0 false pass / 0 false fail** over 24,742
pairs, 297 files. `len(Clauses())` is **84**. Three pre-existing XMP-reader defects filed rather than fixed
(`/pending 654`), each outside this slice's clauses.

**Tier 4 / tier 6 did NOT fire.** The diff touches `internal/uacheck`, `internal/pdfops/labelua.go` (a comment
count), `README.md` and `docs/accessibility-parity.md` — no `internal/server`, `internal/p2p` or
`internal/rendezvous`, so neither `pairrepro.sh` nor `ceremonyrepro.sh` has a subject.

#### P06.S02 — the three one-key refusals: Suspects, an embedded file's names, dynamic XFA *(done 2026-09-23, v1.155.0)*
Scope: `7.1 t4` (`Suspects != true` on the mark-info dictionary), `7.11 t1` (a file specification with an
`/EF` carries non-empty `F` AND `UF`), `7.15 t1` (`dynamicRender != 'required'` on the AcroForm).
Acceptance:
- Each clause's SUBJECT population is stated from veraPDF's source before the rule is written — in
  particular which file specifications `CosFileSpecification` reaches (every one in the document, or only
  the embedded-files name tree), because that decides whether an attachment on an annotation is a subject.
- `7.11 t1`'s conjunction is split: a spec with `F` and no `UF`, with `UF` and no `F`, and with an EMPTY
  string in either, are four fixtures and not one.
- `7.15 t1` reads the XFA's own `dynamicRender`, so a static XFA passes; a document with no AcroForm at all
  ~~is a passing check and not an absent subject, per P05's measured trap~~ **has NO SUBJECT — pin,
  2026-09-23.** Measured before the rule was written: veraPDF reports `passedChecks="0" failedChecks="0"`
  for `7.15 t1` on every corpus document without an AcroForm, which is most of them. The sketch carried
  P05's trap to a clause it does not fit — there the subject was an annotation that EXISTS and is exempt;
  here there is no AcroForm to be a subject at all. `NotApplicable` is the agreeing answer.
- Three measured pass and fail fixtures, three `corpusReach` rows; `len(Clauses())` is 87.

**(grill, 2026-09-23 — *(in progress)*)** The three profile tests, transcribed: `Suspects != true`
(object `CosDocument`), `containsEF == false || (F != null && F != '' && UF != null && UF != '')`
(object `CosFileSpecification`), and `dynamicRender != 'required'` (object `PDAcroForm`).

**`7.11 t1`'s SUBJECT POPULATION is a SIX-HOLDER WALK, and "the embedded-files name tree" is wrong.**
This is the slice's whole risk and it is the P05.S04 media-clip shape again: a holder nib does not walk
is a silent Pass over a file specification nobody looked at. Measured against veraPDF 1.30.2, each on a
hand-built PDF carrying ONE defective spec (`/EF` and `/F`, no `/UF`) reachable only through the holder
under test, **with the stimulus asserted in every build** (the spec's own filename present in the
written bytes):

| holder | veraPDF |
|---|---|
| `/Names /EmbeddedFiles` name tree | **failed**, 2 checks |
| a `FileAttachment` annotation's `/FS` | **failed** |
| the catalog's `/AF` | **failed** |
| a page's `/AF` | **failed** |
| a structure element's `/AF` | **failed** |
| a form XObject's `/AF` | **failed** |

The name-tree spec scores **two** checks for one object — veraPDF reaches it twice and grades it twice.
That is irrelevant to nib, which reports one `Result` per clause, and it is recorded so the count is not
mistaken later for two subjects. `FileSpecificationKeysHelper` names further routes this slice must
measure before it claims the population: appearance streams, an image XObject's `/Mask` and
`/Alternates`, and nested resources.

**Two probes were VACUOUS and are recorded as such rather than as evidence.** An unreferenced `Filespec`
and an `/AF`-borne one built through pdfcpu both came back "passed" — and in both cases pdfcpu had
**dropped the object**, so the check ran with no subject present. Only the asserted stimulus (`spec
present = false`) showed it. The hand-built harness above exists because of that, and the pdfcpu
behaviour is its own finding (below).

**`7.15 t1` DOES fail its corpus fixture** — 1 check on `7.15-t01-fail-a.pdf`. An earlier reading that
it did not was taken from an interleaved four-document report and was wrong; re-measured per file. It is
`NotApplicable` on every document with no AcroForm, which is most of them.

**`7.1 t4`'s object is `CosDocument`, one check per document**, and `Suspects != true` means an ABSENT
`/MarkInfo` or an absent `/Suspects` PASSES. **A non-boolean `/Suspects` is now
measured, and the answer is not the interesting part.** veraPDF PASSES `/Suspects (true)` written as a
STRING (`passedChecks="1"`), so `Suspects != true` means the boolean true and nothing else — which is what
the rule does. **But nib cannot open such a document at all**: pdfcpu's `validateBooleanEntry` errors
rather than deleting, so `Check` emits no report where veraPDF answers Pass. The branch is correct and
unreachable through a file — the same class as `6.1 t1`'s `%PDF-1.9` — and it is tested in memory.

Tasks:
- `T01 — the file-specification population as ONE door, over every measured holder`.
- `T02 — 7.11 t1 over that door, its conjunction split four ways`.
- `T03 — 7.1 t4, including what a non-boolean /Suspects does`.
- `T04 — 7.15 t1 over the AcroForm, reading the XFA's own dynamicRender`.
- `T05 — corpusReach rows, oracle documents reaching both halves, notTreeRules; len(Clauses()) is 87`.

**(review, 2026-09-23 — three reviewers over the diff, packs `go` + `verification`)** Both oracles were
green when the review started. **Every finding below was invisible to them**, and four were live defects.

- **A form XObject is a STREAM.** `types.StreamDict` EMBEDS `types.Dict`, so a type switch with a `Dict`
  case and no `StreamDict` case walks neither. A specification on a form XObject's `/AF` — a holder this
  slice had already MEASURED and written into its own table — was invisible. The holder list was right;
  the holder was missed by TYPE. Probed red: 0 specs found.
- **The walk was an attacker-supplied exponential.** Arrays carried no visited set and spent no budget,
  so `[prev prev]` chained n deep was re-walked 2^n times: n=21 took 198 ms from a 3 KB file. The same
  walk counted depth THROUGH references, so a 64-long chain of linked dictionaries — **an outline with
  64 bookmarks** — refused the whole clause. Both are fixed by one change: indirect references are not
  followed, because the object-table loop reaches every object anyway. Measured after: a 2^30 document
  settles in 0.01 s and the 70-long chain reports its real defect.
- **One unreadable XFA packet refused the form.** Go's `encoding/xml` rejects `encoding="ISO-8859-1"`,
  `version="1.1"` and `&nbsp;` — all ordinary in XFA rich text — so a document whose `template` packet
  carried a named entity answered CannotCheck where veraPDF passes. Packets are now tried in turn, and
  the XFA read carries ceilings (64 packets, 32 MiB) against a flate bomb repeated 20,000 times in one
  `/XFA` array.
- **`dynamicRender`'s text was read in one piece.** `requi<!--x-->red` read as `requi` — a Pass on a
  dynamic form — and a nested element of the same name collapsed the reading.
- **The answer was nondeterministic.** Sorting object numbers left the per-dictionary key loop random:
  40 opens of identical bytes gave two different reasons, 35 and 5.

**Two reviewer findings were REFUTED by measurement rather than accepted.** An unreferenced specification
was argued to be a false fail; veraPDF FAILS a truly orphaned spec, so nib agrees. A non-boolean
`/Suspects` and a name-valued `/F` were argued to be a false pass and a wrong branch; veraPDF passes the
first and fails the second, exactly as nib does — both branches were right and merely untested, and both
are unreachable through a file because pdfcpu refuses such documents outright.

**Red-proof: 14 targeted and 6 blind probes, 7 survivors, all disposed.** Two probes of my own were
themselves defective and caught by their own stimulus assertions — one mutation left the live call intact
beside a dead one, and one fixture's ordering depended on object numbers a test cannot control.

**Gates.** Oracle **6,786 of 6,786** over 78 documents; corpus **0 false pass / 0 false fail** over 25,627
pairs. `len(Clauses())` is **87**. `/pending 655` and `656` filed.

**Tier 4 / tier 6 did NOT fire** — the diff is `internal/uacheck` plus a comment in `internal/pdfops`, two
docs and the plan; no `internal/server`, `internal/p2p` or `internal/rendezvous`.

#### P06.S03 — encryption's permission bit, and the reference XObject *(done 2026-09-23, v1.156.0)*
Scope: `7.16 t1` (an encrypted file's `/P` has bit 10 set) and `7.20 t1` (no reference XObjects).
Acceptance:
- `7.16 t1` is NotApplicable for an unencrypted document and a definite Fail for an encrypted one whose
  bit 10 is clear — the two are different answers and the fixture set proves it.
- **The phase records that nib's own `Encrypt` output fails this clause**, with the measured `/P` value, and
  files nothing new: `/pending 640` owns the writer. The rule does not pick permissions for it.
- `7.20 t1`'s subject is a Form XObject carrying `/Ref`; the population is stated from veraPDF's source,
  since `PDXForm` is reached through resources that may nest.
- Two measured pass and fail fixtures, two `corpusReach` rows; `len(Clauses())` is 89.

**(grill, 2026-09-23 — *(in progress)*)** The two profile tests: `P != null && (P & 512) == 512` on the
object `PDEncryption`, and `containsRef == false` on the object `PDXForm`.

~~**`7.20 t1`'s population is the SAME object graph `7.11 t1` walks.**~~ **REFUTED BY MEASUREMENT —
pin, 2026-09-23.** The first three measurements all drew the form, so they could not see the property
that matters. Re-measured: **a form XObject is a subject only when the document DRAWS it.**

| document | veraPDF |
|---|---|
| a page-resources form, DRAWN by `/X0 Do` | **failed** |
| the same form, in resources and never drawn | **passed, 0 checks — NO SUBJECT** |
| a form an outer form DRAWS | **failed** (outer passes, inner fails) |
| the same inner form, in the outer's resources but not drawn | **passed**, outer alone |
| an annotation's `/AP` in `/N`, `/R` or `/D` | **failed**, each |

**So the population is the CONTENT WALK, not the object-graph door**, and the two clauses deliberately
do NOT share one: `7.11 t1`'s subject is every dictionary of a shape wherever it hangs, and `7.20 t1`'s
is what the page actually paints. The object-graph reading would report a failure veraPDF does not — a
false FAIL, which for a checker is as bad as a missed one. **The oracle caught it on the very document
added to reach this clause's failing half**, which is the whole argument for adding both halves before
believing a population.

This is S02's lesson in the mirror: there, enumerating holders was too NARROW; here, the object graph
is too WIDE. Neither error is visible from the clause's wording.

**`7.16 t1`'s subject is the ENCRYPTION DICTIONARY, and the corpus cannot supply its passing half.**
Measured: `7.16-t01-fail-a.pdf` fails with one check, and `7.16-t01-pass-a.pdf` reports **0 passed / 0
failed — no subject at all**, because it carries no encryption dictionary. So the corpus has NO document
where the clause passes WITH a subject, and the oracle must supply one or the reach row records the zero
with its reason, as `7.18.2 t1` does.

**Both encrypted fixtures OPEN in nib** — measured, pdfcpu handles the empty-password case — so unlike
`6.1 t1`'s `%PDF-1.9` this clause is reachable through a file.

**The declared coupling, unchanged from the phase open**: nib's own `Encrypt` writes `/P = -3901 =
0xF0C3`, and `0xF0C3 & 512 == 0`, so **nib's own protected output fails this clause**. The slice records
that with the measured value and files nothing new — `/pending 640` owns the writer, and which
permissions `Encrypt` should grant is Dan's call, not a defect with one right answer.

Tasks:
- `T01 — lift filespecs.go's descent into one door` *(done; `7.11 t1` is its only caller — see the pin)*.
- `T02 — 7.20 t1 over the CONTENT walk, across all four measured holders`.
- `T03 — 7.16 t1 over the encryption dictionary, with the bit-10 test and the no-subject case`.
- `T04 — record that nib's own Encrypt output fails it, with the measured /P`.
- `T05 — corpusReach rows, oracle documents for both halves, notTreeRules; len(Clauses()) is 89`.

**(review, 2026-09-23 — one reviewer over the diff, packs `go` + `verification`)** The reviewer
confirmed the population change and found one real defect plus a set of contract and fixture problems.
**Two of its findings were REFUTED by measurements it named as missing**, and both measurements are now
tests: a form an outer form DRAWS is a subject while one merely in the outer's resources is not, and all
three appearance states — `/N`, `/R` and `/D` — are graded, not just the normal one.

- **The drawing walk silently dropped a form it could not read.** Since this slice it is also
  `7.20 t1`'s population, so an XObject that fails to resolve removed a SUBJECT with no refusal and the
  clause answered Pass. Now recorded — and the branch is declared unreached, because pdfcpu refuses a
  non-stream `/XObject` ahead of the rules while veraPDF grades the document, which is a divergence of
  `6.1 t1`'s class.
- **Four doc comments and this plan's own holder table asserted the refuted design** — that both clauses
  share the object-graph door. Corrected at every site; the table above carries the pin.
- **`withRefOnAppearance` stamped every form in the file**, not the appearances its test names, so a
  Fail could have come from page content and nothing asserted otherwise. It now goes through the
  annotation door.
- **One bit-table case measured nothing**: `-3901 &^ 512` is `-3901`, since bit 10 is already clear
  there. Replaced with `-1 &^ 512`, which is the value the case claimed to be.
- The `7.11 t1` refusals had lost their specificity in the lift, and a drawn form's `Where` discarded
  the walker's location; both restored.

**Red-proof: 6 targeted probes, 2 survivors.** One was a real hole — the bit mask, which neither
`PermissionsAll` nor `PermissionsNone` can isolate — now pinned by `/P` values that set bit 10 alone,
bit 9 alone and bit 11 alone. The other is the declared unreached refusal above.

**Gates.** Oracle **7,209 of 7,209** over 81 documents; corpus **0 false pass / 0 false fail** over
26,217 pairs. `len(Clauses())` is **89**. Live-verified through the real binary on all six measured
holders.

**Tier 4 / tier 6 did NOT fire** — the diff is `internal/uacheck` plus a comment in `internal/pdfops`,
two docs and the plan.

#### P06.S04 — Formula, and the counterexample it retires *(done 2026-09-23, v1.157.0)*
Scope: `7.7 t1` — an `SEFormula` carries a non-empty `/Alt` or an `/ActualText`. One rule, its own slice,
for the coupling above. Refs: `counterexample_test.go`, ADR-031 law 1.
Acceptance:
- The predicate matches veraPDF's exactly: `(Alt != null && Alt != '') || ActualText != null` — so an EMPTY
  `/Alt` fails while an empty `/ActualText` PASSES, which is an asymmetry to measure rather than tidy.
- `TestADocumentCanPassEveryClauseNibChecksAndStillFailVeraPDF` is made to pass again **by a new
  counterexample that veraPDF is measured to fail and nib is measured to pass**, not by weakening or
  deleting the test. If no such document can be found from the remaining 15 unbuilt rules, that is a
  finding about the docs' sentence and it is reported rather than worked around.
- Every prose site citing the formula example moves with it: `README.md`, `internal/uacheck/door.go`,
  `internal/uacheck/rules_catalog.go`, `docs/accessibility-parity.md`.
- A measured pass and fail fixture and a `corpusReach` row; `len(Clauses())` is 90.

**(grill + review, 2026-09-23)** One rule, and its predicate already existed: `7.3 t1` (Figure) has the
IDENTICAL profile test, so `hasAlternateText` is now one door (ADR-009) and `7.3 t1`'s corpus reach of
18 is unmoved — which is the evidence the extraction changed nothing.

**The asymmetry is reproduced, not tidied**: `(Alt != null && Alt != '') || ActualText != null` means an
EMPTY `/Alt` FAILS while an EMPTY `/ActualText` PASSES. Both probed red.

**The slice's real work was the counterexample, and it went badly before it went well.** The test is
built to go red when a rule catches its example, and `7.7 t1` caught it — the second time, after
`/pending 487` caught a skipped heading level. The replacement hunt produced a **critical finding**: a
nib document whose `/ToUnicode` destination is rewritten to `<0000>` is conformant to nib and failed by
veraPDF, but on `7.21.7 t1`, **a clause nib implements** — veraPDF's object there is `Glyph` and its
test `toUnicode != null`, i.e. per USED GLYPH, where nib asks only whether the font has a mapping
mechanism. That is a live false pass in a shipped rule, filed `/pending 657` for P07. Using it would
have rested the docs' central honesty claim on a checker bug, so the test now asserts the example fails
**only** clauses nib does not implement.

**The review found a second live false pass, in the door this slice had just extracted.** `/ActualText`
was read straight from the map while `/Alt` resolved, so a reference to a missing object counted as
present: measured, a Formula whose only alternate text was `/ActualText 9999 0 R` PASSED nib and FAILED
veraPDF, while the same dangling reference on `/Alt` was failed by both. Pre-existing in `7.3 t1` and
extended to `7.7 t1` by the extraction — which is also what put the two halves side by side where the
asymmetry was visible. Fixed and probed red on both keys.

**Two claims of mine were false and are corrected rather than quietly dropped.** The phase open said
FOUR prose sites cite the formula example and named two that never did; the real set is SIX, and three
(`internal/cli/commands.go`, `cli_test.go`, `door_test.go`) were missed on the first pass — one of them
asserting "which nib does not check" about a clause this slice implements. And the new test's first
draft justified abandoning a buildable counterexample by claiming every remaining clause is a font rule;
`7.20 t2` is not, and nib's own `pdfops` tests record producing a document that fails it. The honest
reason is narrower: `7.20 t2` is **P06.S05**, so an example built on it would be retired within the week.

**Gates.** Oracle **7,290 of 7,290** over 81 documents; corpus **0 false pass / 0 false fail** over
26,511 pairs. `len(Clauses())` is **90**. Live-verified on all five of veraPDF's own `7.7` fixtures.
Tiers 4 / 6 did NOT fire.

#### P06.S05 — a Form XObject's content has one semantic parent *(done 2026-09-23, v1.158.0)*
Scope: `7.20 t2` — `isUniqueSemanticParent`. Its own slice because it is the only clause in this family
that is a GRAPH property over the structure tree rather than a key read, and because ADR-038 built the
write side of exactly this machinery. Refs: ADR-038, `/Stm` as a marked-content reference's key; P02.S08.
Acceptance:
- The predicate is read from veraPDF's source and stated: what makes a semantic parent, and what makes it
  non-unique — a form drawn twice on one page, a form drawn on two pages, and a form whose MCIDs are
  claimed by elements under different parents are three different documents and each is measured.
- nib's own n-up output is checked against it, since ADR-038's origin was a carry that scored identically
  broken and repaired on every instrument the repo had.
- A measured pass and fail fixture and a `corpusReach` row; `len(Clauses())` is **91**, completing P06's
  rule set.

**(grill, 2026-09-23)** **The predicate reads no MCID and no structure tree.** veraPDF 1.30.2's
`GFPDXForm.getisUniqueSemanticParent` (`GFPDXForm.java:163-176`) is: no `/StructParents` KEY → pass; no object
key → pass; object key already in `XFormKeysSet` → FAIL; else add it and pass. `knownKey` is key PRESENCE — a
`/StructParents 30 0 R` naming `null`, or naming nothing, is a key (both measured FAILED twice-drawn), while a
direct `null` is not (measured passed). The message's *"contains MCIDs"* is prose, not the test: a form with
MCIDs and no `/StructParents` drawn twice PASSES, one with `/StructParents` and no MCIDs FAILS, and a form whose
MCIDs are claimed by elements under two different parents PASSES — the acceptance's third document is measured
and it is not a subject of this clause at all.

**The tally is veraPDF's TRAVERSAL, and that is the slice.** A `GFPDXForm` is built at every `Do` and at every
annotation appearance entry, and has no id, so each is checked; but a content stream HAS one (its object key),
and veraPDF's validator visits an object with an id once. So a `Do` counts only in the first traversal of the
stream that holds it. Measured, 38 documents (31 at the grill, 7 from the review and red-proof): an outer form drawn twice whose inner form carries the key PASSES
(3 checks, the inner once); a tiling pattern used twice PASSES while one whose content draws the form twice
FAILS; two pages sharing ONE content stream PASS while two sharing it through an `/Contents` ARRAY (no key) FAIL;
a Type 3 font shown twice PASSES while one whose two glyph procedures each draw the form FAILS although only one
glyph is shown; a form drawing itself FAILS (the self-`Do` is the second reach); one annotation on two pages
PASSES (the annotation has an id); two annotations sharing an appearance, `/N` and `/D` naming one stream, and a
hidden or `/Popup` annotation beside a `Do`, all FAIL.

**pdfcpu's optimize pass FUSES equal form XObjects** (`optimize.go:556-593`, `EqualObjects`), which is the
ADR-038 origin seen from the reader: two identical forms each carrying `/StructParents 0`, each drawn once, are
PASSED by veraPDF and read by nib as one form drawn twice — a false FAIL — and two identical outer forms each
drawing a keyed inner form are FAILED by veraPDF and read by nib as one traversal — a false PASS. The fused
duplicates stay in the xref table, measured, so the fusion is detectable with pdfcpu's own predicate: a form
with no twin is exact; a keyed form reached more times than it has twins is a definite FAIL (pigeonhole); any
other reached form with a twin is `CannotCheck`, never a verdict on a count nib knows is not veraPDF's.

- T01 — the walk tallies reaches by object number and traverses each keyed stream once for the tally
  (`repeat` on the walker; page `/Contents`, forms, appearances; patterns and glyph procedures already once).
- T02 — `7.20 t2` over the tally, with the twin refusal; `len(Clauses())` 91.
- T03 — measured fixtures for every shape above as a standing test; `corpusReach` and population rows.
- T04 — nib's own n-up output into the live oracle, and the fused-carry mutation beside it.

**(review + red-proof, 2026-09-23)** The diff review found **two live defects the 31 measured documents could not
show**, both in the population rather than the predicate: pdfcpu's `EqualObjects` is ASYMMETRIC on a null
(`o1 == nil → o2 != nil`), so a twin asked one way round was missed — a false FAIL on 27 of 40 runs of a file
veraPDF passes, the verdict riding on Go's map order; and annotations were deduplicated by map identity, which a
DIRECT annotation in an `/Annots` array two pages share also has — a false PASS, since veraPDF gives a keyless
object a fresh id on every visit. A third, environment-dependent: `NewDefaultConfiguration` reads the user's own
pdfcpu config, and `optimizeDuplicateContentStreams: true` there fused two pages' content into one traversal.
All three fixed and probed red. **The red-proof then found a fourth**: its two survivors could only be reached by
a form inheriting its page's resources, and building that shape showed pdfcpu's reader DROPS the inherited
`/Pattern` binding while `enterPattern` returned without a word — a false PASS on a file veraPDF fails. Now a
refusal, like the XObject route already was, and the same documents measured veraPDF keeping the FIRST
traversal's binding, which settles the gap the grill left open. Probes: 21 targeted (19 red, the two survivors
declared unreachable through a file), 5 blind (5 red).

**Out of scope and filed**: nib's own Markdown conversion past ~35 sections has veraPDF typing fewer headings than
it writes and `7.4.4 t3` losing its subject (`/pending 658`) — found because a 40-section n-up source disagreed on
that clause, so the oracle's n-up is 20 sections; and every other content rule still grades a repeated stream
veraPDF traverses once (`/pending 659`). `pdfops`' completeness condition 4 claimed to BE `7.20 t2` and keys on
MCIDs; its comment now says it is a neighbour.

**Acceptance ledger (2026-09-23).** Predicate read from source and stated — MET (`GFPDXForm.java:163-176`; the
key, never an MCID). Non-uniqueness stated — MET (a second `PDXForm` for one object key; once per traversed
stream). Drawn twice on one page — MET (veraPDF 1/1 failed, nib Fail). Drawn on two pages — MET (1/1, Fail).
MCIDs claimed under two parents — MET (veraPDF passes, 1 check; not a subject of the failing half). nib's own n-up
checked — MET (live: n-up passed 4 checks / nib Pass; the fused carry failed 3/1 / nib Fail; both in the oracle).
Measured pass and fail fixture — MET (corpus `7.20-t02-pass-a`/`-fail-a` via the real `nib ua`, and 34 table rows).
`corpusReach` row — MET (42). `len(Clauses())` 91 — MET (`TestPassingEveryClauseNibChecksIsNotConformanceAndTheDocsSaySo`
reads "91 of the 106" from the registry). P06's rule set complete — MET (all eleven registered). **Gates**: oracle
7,735/7,735 over 85 documents; corpus 0 false pass / 0 false fail over 26,806 pairs; tiers 4 and 6 did NOT fire (no
session, ceremony, delivery, discovery, p2p or rendezvous path in the diff).

### P07 — Checker: font programs (~12 rules)
**Goal.** CMaps, CIDToGIDMap, width agreement, TrueType cmap and encoding, CharSet, `.notdef`, ToUnicode values.
**Exit criteria.** As P03, and a font program nib cannot parse returns `CannotCheck` naming its type, never
`Pass`. Refs: law 1. Residual doubt: `/pending 493`.

**(phase-open, 2026-09-23, v1.158.1)** The family is **fourteen** font rules — counted against veraPDF 1.30.2's
`PDFUA-1.xml` and `Clauses()` (91 registered, 15 unbuilt; the fifteenth is `7.1 t12`, still declared and not
registered, per P03's close). Landing all fourteen takes the checker to **105**.

**`/pending 493` answered at this open, measured: hand-written readers, no library.** `golang.org/x/image`'s
`font/sfnt` is already a direct dependency and refuses **7 of the corpus's 10** distinct TrueType programs (five
on unaligned table offsets, one with no `cmap`, one on `post`); it exposes neither the cmap subtable count nor
(3,0)'s presence, and reads no bare CFF and no Type 1. pdfcpu parses `cmap`/`hmtx` unexported from a file path, with
no `glyf`, CFF or Type 1. What nib HAS is `pdfops/textrun.go`'s per-font code splitting and ToUnicode parsing
(`loadRunFont`, `splitCodes`, `parseToUnicode`) — uacheck does not import pdfops, so reuse is a shared door
(ADR-009), not a copy.

**Two couplings found at the open.**
- **The per-glyph population does not exist in uacheck**: `contentEvent` carries the font and the render mode but
  no string operands. Five clauses stand on it (7.21.4.1 t2, 7.21.5 t1, 7.21.7 t1/t2, 7.21.8 t1), and so does
  `/pending 657`'s CRITICAL — the SHIPPED 7.21.7 t1 is per-FONT where veraPDF's object is `Glyph` (the corpus's
  `7.21.7-t01-fail-a` fails **8** checks). So the door is built once, in S02, and 657 closes there.
- **veraPDF answers differently from nib when a program will not parse**: `GFGlyph` leaves presence and width null
  (so 7.21.4.1 t2 and 7.21.5 t1 PASS), while `GFPDTrueTypeFont`/`GFPDType1Font` return FALSE (so 7.21.6 t2 and
  7.21.4.2 t1 FAIL). The exit criterion's `CannotCheck`-never-`Pass` is stricter than both; the oracle will show it
  as a refusal, which is permitted, and each slice records which way veraPDF goes.

The corpus holds **46** files for the family; seven clause halves have NO corpus file (7.21.4.1 t2 both, 7.21.6
t1/t4 both, 7.21.8 t1's pass) and **no Type 1 `FontFile` program exists in it at all**.

`/plan-review` did NOT fire: read-only clauses over the document, no format, migration or egress change.

#### P07.S01 — composite fonts' CMaps, at the dictionary *(done 2026-09-23, v1.159.0)*
Scope: `7.21.3.1 t1` (CIDSystemInfo against the CMap), `7.21.3.2 t1` (a CIDFontType2 with an embedded program has a
`CIDToGIDMap`), `7.21.3.3 t1`/`t3` (a CMap, and a CMap it references, is one of ISO 32000-1 Table 118's predefined
names or embedded), `7.21.3.3 t2` (an embedded CMap's `/WMode` agrees with its program's).
Acceptance:
- Each predicate transcribed from the profile, and where veraPDF gets the CMap's Registry/Ordering/Supplement
  (stream or predefined table) READ from its source or measured, not assumed.
- The corpus's 12 files agree; the predefined-name table carries its provenance (Table 118).
- `len(Clauses())` 96.

**(grill, 2026-09-23)** Every predicate was read from veraPDF's source, and veraPDF-parser 1.30.2 was fetched to read
the half the validation model does not hold.
- **A CMap's collection**: a named one's comes from `CharacterCollections`, a table holding THREE columns (PDF 1.4,
  ISO 32000-1, ISO 32000-2). PDF/UA-1 reads the ISO 32000-1 column, **measured**: `GB-EUC-H` is Adobe-GB1-0 there
  and -5 in the next, and a CIDFont at supplement 2 fails. A stream CMap's comes from its own `/CIDSystemInfo`. A name
  outside the table has an EMPTY collection, which fails.
- **`containsFontFile` is not the key's presence** but "a program exists AND veraPDF parsed it"
  (`GFPDFont.java:171-173`), so veraPDF PASSES 7.21.3.2 for an embedded CIDFontType2 it cannot parse. nib fails only a
  program its own TrueType reader opens, and refuses the rest.
- **A referenced CMap is a CMap**: `GFPDReferencedCMap` extends `GFPDCmap`, so `/UseCMap` chains, 7.21.3.3 t1 applies
  down the chain (embedded escape included), and t3 applies only there (no embedded escape).
- **The corpus's own fail files were FALSE PASSES on the first run.** pdfcpu's validator DROPS a Type 0 font whose
  CIDFontType2 lacks a CIDToGIDMap, so on `7.21.3.2-t01-fail-a`/`-c` the text names a font nib's reader has nothing
  under. The population now refuses an unresolved font, as the shipped font rules already do; recovering it is
  /pending 656's one raw door.
- **veraPDF cannot be asked about one shape**: a `/UseCMap` naming an unknown CMap throws a NullPointerException in
  veraPDF 1.30.2 and reports nothing, so it is pinned to the predicate.
- **pdfcpu refuses a CMap stream whose `/CIDSystemInfo` is PDF 1.4's array form**, so that branch is declared
  unreachable through a file.
- **Adding the oracle documents found three shipped-rule divergences on an untagged page**: 7.21.4.2 t2 (a
  non-embedded CID font is a subject that passes — fixed), and 7.1 t11 on an empty root and 7.2 t30-t32 with no
  marked content (/pending 674 — 7.1 t11's strictness is ADR-031's and is parked for Dan).

- T01 — the five rules over one population door (`type0Fonts`), the predefined table transcribed from source.
- T02 — measured fixtures and oracle documents for every half; `corpusReach` and population rows.
- T03 — the count moves to 96 in every prose copy.

**(review + red-proof, 2026-09-23)** Two review rounds found **four** live disagreements, all in what nib reads
rather than what it tests: a CIDFontType2 program is `/FontFile2` OR any `/FontFile3` (two false passes, one per
subtype the review tried), parsed by its declared SUBTYPE (the parent's own measurement refuted the first fix: TrueType
bytes under `/CIDFontType0C` are PASSED by veraPDF, which reads them as CFF and has no parsed program); a `/WMode`
inside a `{ }` procedure never runs; and an unclosed procedure makes veraPDF's parser throw and keep WMode 0. A
`/UseCMap` cycle kills the whole process inside pdfcpu's validator — reproduced, filed CRITICAL as /pending 675. 16
targeted mutations, 16 red.

**Acceptance ledger.** Each predicate transcribed from the profile — MET. Where veraPDF gets the CMap's collection
READ from source (veraPDF-parser `PDCMap.getCIDSystemInfo`) and the ISO 32000-1 column MEASURED — MET. The corpus's 12
files agree — MET for 10 scored both ways; `7.21.3.2-t01-fail-a`/`-c` are refused, not scored, because pdfcpu drops
the font the clause fails (declared, /pending 656). The predefined table carries its provenance — MET (every row cites
its source triple). `len(Clauses())` 96 — MET. Oracle 8,832 of 8,832 over 92 documents; corpus 0 false pass / 0 false
fail. Tiers 4 and 6 did NOT fire.

#### P07.S02 — the glyph population, and 7.21.7 per glyph *(done 2026-09-23, v1.160.0)*
Scope: a per-glyph door over every text-showing operator — codes split by the font's codespace, each with its
font, render mode and ToUnicode value — shared with `pdfops/textrun.go` (ADR-009); `7.21.7 t1` rebuilt on it
(closing `/pending 657`) and `7.21.7 t2` (ToUnicode values never U+0000, U+FEFF or U+FFFE).
Acceptance:
- `7.21.7-t01-fail-a` fails the same glyphs veraPDF counts; `/pending 657`'s `<0000>` document goes red on 7.21.7 t1.
- A composite font whose codespace nib cannot split, and a non-Identity ordering with no ToUnicode, are `CannotCheck`
  naming why — never a Pass over glyphs nib did not read.
- The walk's existing budgets bound the new population (a string operand is attacker-sized).
- `len(Clauses())` 97.

**(grill, 2026-09-23 — premise amended, slice split)** veraPDF 1.30.2's source was read for the whole `Glyph`
path (validation-model `GFOpTextShow`/`GFGlyph`, parser `CMap`/`PDCMap`/`PDSimpleFont`/`PDType0Font`/`AdobeGlyphList`).
- **`/pending 657`'s `<0000>` premise does not reproduce, measured.** Rewriting the first `bfchar` DESTINATION to
  `<0000>` in nib's own Markdown conversion makes veraPDF fail **7.21.7 t2** (2 checks), not t1: a destination is
  `new String(bytes, UTF_16BE)`, so `<0000>` is `"\u0000"`, never null. t1 fails when the code has NO mapping —
  rewriting the SOURCE does that. The first acceptance clause is read as: the `<0000>`-destination document goes red
  on **t2**, and a document whose drawn code is unmapped goes red on t1.
- **The population**: `Tj`, `TJ` and `'` only — `"`'s argument 0 is a number, so it yields no glyph; invisible text
  counts; page content, forms, the N/D/R appearances (`GFPDAnnot.java:414-426`), tiling patterns and Type 3 procedures.
  Codes are read by the Encoding CMap's codespace (`CMap.getCodeFromStream`), and ToUnicode is keyed by the code's
  INTEGER value. A glyph is checked once per (font, code, Tr, marked content, element).
- **The fallbacks need data nib does not carry.** A Type 0 font under an Adobe Japan1/CNS1/GB1/Korea1/KR ordering
  falls back to Adobe's UCS2 CMaps, and a non-Identity predefined CMap needs its codespace — both `CannotCheck` naming
  why (filed). A simple font falls back to its encoding's glyph name through **veraPDF's own** Adobe Glyph List, which
  maps `.notdef` to U+0000 and `zerowidthjoiner` to U+FEFF — so a glyph without a ToUnicode entry can FAIL t2, and
  neither clause may pass one on a guess.
- **Split at the grill, and folded back at T03, measured.** The grill cut the simple-font fallback out as S02b;
  registering t2 without it made **forty-odd of the oracle's own documents** — every one drawn in Helvetica —
  refuse t2, because a standard face's codes reach U+0000 through `.notdef` and nib could not say which. So the
  encoding path landed here: veraPDF's encoding construction (a NAMED encoding is MacRoman, MacExpert or WinAnsi
  and nothing else — `/StandardEncoding` named in a font is EMPTY; a dictionary's `/Differences` over its
  `/BaseEncoding`; a standard Type 1 face with no descriptor reads its metrics' scheme), Adobe's Glyph List with
  veraPDF's one addition, and the Symbol/ZapfDingbats name sets. A name only a font program could give is
  `CannotCheck` naming the program's type.
- **The slice's own fixture found a live strict disagreement in SHIPPED 7.21.4.1 t1**: text drawn only in `3 Tr`
  is a subject that PASSES (`renderingMode == 3`), and nib answered NotApplicable — measured on Helvetica and
  on a non-embedded TrueType face, fixed here.

- T01 — `internal/fontcode` (ADR-052): string operands, veraPDF's `/ToUnicode` reading beside the lenient one
  `pdfops` keeps, the codespace code reader; `pdfops/textrun.go` moved onto it.
- T02 — the glyph population in uacheck's walker, deduplicated per (font, code), bounded by `overBudget`.
- T03 — `toUnicode` per glyph, three-valued (mapped / null / unknown with a reason), the Type 0 and simple-font
  fallbacks; 7.21.7 t1 rebuilt, t2 registered.
- T04 — 32 veraPDF-measured glyph fixtures and the two `/pending 657` shapes in the live oracle, corpus reach,
  the knownCannotCheck reasons, the count to 97.

**(review + red-proof, 2026-09-23)** Four review rounds (seven reviewers), 39 findings, all dispositioned; the fix pass
found more than the slice's first draft did, and every one was measured on veraPDF before it was pinned.
- **Eight live false passes closed**, each now a fixture: a font rebound in a form was judged by NAME at the show (veraPDF
  binds it at `Tf`); a /ToUnicode stream whose `/CMapName` is Identity-H is identity; a /ToUnicode dictionary's
  `/UseCMap` overwrites its entries; an ExtGState `/Font` rebinds the font (only a dictionary of a type veraPDF builds);
  a tiling pattern inherits its invoking stream's STARTING font and a Type 3 procedure its own; and SHIPPED 7.21.4.1
  t1 called a program embedded on any key's presence — a Type 1 font reads /FontFile or /FontFile3, a TrueType
  /FontFile2 or /FontFile3, a CIDFont what `PDCIDFont` opens.
- **Two attacker-reachable costs bounded**: a linear range lookup (~2.25 ms per code over 2^20 ranges) is a per-256-code
  block index under one document budget, parsed once per stream; and the distinct (font, code) population is capped —
  16.2 s / 10.7 GB became 0.4 s.
- **A panic in the CMap parser** (an empty range code) had left the content population half built for every later rule;
  it refuses now, and a walk that did not return reports as unread.
- Filed: /pending 676 (UCS2 and predefined CMaps not carried), 677 (a program is "embedded" without being parsed),
  678 (fonts used only in patterns / Type 3 procedures are outside the FONT population).
- **Red-proof**: 38 mutations (30 targeted, 8 blind — the blind agent read no test), 38 red after closing three survivors
  (nested TJ arrays and an in-program UCS2 `usecmap` gained fixtures; a dead `prev == 0 ||` conjunct and an inert Type 3
  font assignment were deleted).

**Plan divergence, stated.** The third acceptance clause assumed the walk's EXISTING budgets bound the population; they
did not — one `Tj` is one operator whatever its string holds — so three ceilings were added (`maxGlyphCodes`,
`maxDistinctGlyphs`, `maxToUnicodeBlocks`). T01's "pdfops moved onto it" holds for strings, names and the ToUnicode read;
`pdfops` keeps its own lenient Identity split (ADR-052), because veraPDF's 0xFF completion invents a glyph a text
extractor must not read. The binding, inheritance and 7.21.4.1 fixes are outside the plan's scope and inside the slice's
door: the slice's own fixtures reached them.

**Acceptance ledger.**
- `7.21.7-t01-fail-a` fails the same glyphs veraPDF counts — MET (`TestTheFailingGlyphsAreTheOnesVeraPDFCounts`: codes 3,
  40, 55, 68, 69, 76, 79, 82, exactly veraPDF's eight).
- `/pending 657`'s `<0000>` document goes red — MET as pinned at the grill: on **t2** (the destination is U+0000), and the
  drawn-code-unmapped shape on t1; both are oracle documents on nib's own conversion, and `nib ua` agrees on the CLI.
- A composite font whose codespace nib cannot split is `CannotCheck` naming why — MET (GB-EUC-H rows: "a predefined CMap
  whose codespace nib does not carry").
- A non-Identity ordering with no ToUnicode is `CannotCheck` naming why — MET where veraPDF's answer needs a UCS2 CMap
  (named in the reason); where veraPDF has none, nib FAILS as veraPDF does (Adobe-Identity, measured).
- Never a Pass over glyphs nib did not read — MET (an unread string, a malformed or truncated CMap, and an unfinished walk
  each refuse; `TestAWalkThatDidNotFinishIsNotAWalkThatFoundNothing`).
- The budgets bound the new population — MET by the three new ceilings, each with a control-then-refusal test.
- `len(Clauses())` 97 — MET.
- (S02b, folded in) `7.21.7-t01-pass-b` and `-c` agree — MET; a `.notdef` name fails t2 and not t1 — MET, measured.
- Phase exit: a program nib cannot parse is `CannotCheck` naming its type — MET for the glyph fallback (names the program
  type); parse-ability of an "embedded" program is /pending 677.
- Oracle 14,000+ pairs over 148 documents agree strictly (52 glyph fixtures, 3 declared refusals); corpus 0 false pass /
  0 false fail over 28,567 pairs, 7.21.7 t1/t2 reach 282/282. Tiers 4 and 6 did NOT fire: the slice touched
  `internal/uacheck`, `internal/fontcode` and `pdfops/textrun.go` — none of the session, ceremony, delivery, discovery,
  p2p or rendezvous paths.

#### P07.S03 — the TrueType program, at the font *(done 2026-09-23, v1.161.0)*
Scope: `7.21.6 t1`-`t4` — a TrueType reader grown from `truetype.go` (table directory tolerant of unaligned offsets,
the cmap subtable list), and the symbolic flag against `/Encoding`.
Acceptance:
- t2 and t3 agree on the corpus (4F/4P and 1F/1P); t1 and t4 have no corpus file, so each half is a fixture veraPDF
  was run on, and the zero corpus reach is recorded with its reason.
- A program the reader refuses is `CannotCheck` naming "TrueType".
- `len(Clauses())` 101.

**(grill, 2026-09-23 — premise amended, scope widened by /pending 677)** Every claim of the pre-slice deep-dive was
measured on veraPDF 1.30.2 over 111 hand-built documents (a synthetic sfnt generator); the grill is
`grills/2026-09-23-p07s03-the-truetype-program.md`.
- **The first acceptance clause's premise is wrong**: all ten 7.21.6 corpus files evaluate t1 and t4 (and pass) — only
  their FAIL halves have no corpus file. Read as: t1/t4's fail halves are measured fixtures, and their corpus reach
  is whatever the corpus measures, not zero.
- **veraPDF parses lazily and bounds reads by the program, not the table**: a failed parse leaves the program object
  (so t2's Differences test is FALSE, not skipped), a truncated subtable followed by other tables parses, and a
  program of 10240 bytes or more whose seek runs past its end throws an exception veraPDF does not catch — it reports
  nothing for the document, and nib refuses.
- **The program cache is keyed on the encoding's OBJECT key**, so fonts with direct encodings share one parse and the
  first opened decides it — measured order-dependent on 7.21.4.1 t1. nib emulates the order-independent half and
  refuses the rest.
- **/pending 677's TrueType half lands here** (it names S03 as its closer): 7.21.4.1 t1 passed every broken TrueType
  program veraPDF fails, and the glyph fallback refused where veraPDF answers from the program's own name table.
  CIDFontType2's program stays with S04.

- T01 — `readTrueType`: veraPDF's `TrueTypeFontParser` and `TrueTypeFontProgram.parseFont`, three outcomes, budgeted.
- T02 — `rules_truetype.go`: one population door (`trueTypeFonts`) and 7.21.6 t1-t4.
- T03 — /pending 677's TrueType half: 7.21.4.1 t1 and the glyph fallback ask the door.
- T04 — the measured fixtures as a table and in the live oracle; corpus reach, knownCannotCheck, the count to 101.

**(review + red-proof, 2026-09-23)** Five review rounds (six reviewers), every finding dispositioned in
`code-reviews/v1.160.0-p07s03-2026-09-23.md`; each divergence re-measured on veraPDF before it was pinned.
- **Seven live disagreements closed**: /Flags written as a real (veraPDF casts it); a /FontFile3 program's name-table
  throw leaves EVERY sharing font unparsed; the OpenType cache key's subset flag is "six characters before the first
  plus"; a /Differences code and its running index are Java ints (a wrap past 2^32 re-targets a code, a step past 2^31
  makes veraPDF report nothing); an unrelated unresolved font had silenced definite failures on 7.21.4.1 t1 and 7.21.6
  t3; and where an unresolved font could be opened first, an OpenType group and the fallback names refuse.
- **One attacker-reachable cost bounded**: a program is parsed once per stream against one document read budget (200
  fonts on one program cost 3.4 s before; 400 now cost one parse).
- **Deferred**: 7.21.4.1 t1 reads a font's FIRST use's render mode in veraPDF and ANY visible use in nib — pre-existing,
  every font type, /pending 679; a /Flags beyond 2^63 reaches nib as 0 through pdfcpu's parser, /pending 656.
- **Red-proof**: 45 mutations (37 targeted, 8 blind — the blind agent read no test); three blind survivors and five
  targeted ones closed with measured fixtures, one inert branch (the `.notdef` refill) deleted.

**Plan divergence, stated.** Scope widened by /pending 677's TrueType half (7.21.4.1 t1 and the glyph fallback ask the
parse) and by `glyphnames.go`'s `differences` (the Java-int reading every simple font shares). The first acceptance
clause's premise was wrong, pinned at the grill. `embeddedProgram` is now Type 1's only.

**Acceptance ledger.**
- t2 agrees on the corpus — MET (4 fail files, all agree strictly; reach 255).
- t3 agrees on the corpus — MET for the readable files (reach 255); its one fail file is unreadable to pdfcpu
  (`corpusUnreadable`), so the fail half is measured fixtures (six shapes).
- t1 and t4's fail halves are fixtures veraPDF was run on — MET (t1: 4 fail shapes, t4: 15, and 40 no-subject shapes each); their pass halves reach the
  corpus at 255, so "zero corpus reach" is recorded as the grill's corrected premise, not a zero.
- A program the reader refuses is `CannotCheck` naming "TrueType" — MET: every unknown outcome reads "whether veraPDF
  parses its TrueType program is not known"; a program veraPDF itself fails to parse is NOT a refusal but no subject
  (t1/t4) and not embedded (7.21.4.1 t1), as veraPDF answers.
- `len(Clauses())` 101 — MET.
- Phase exit (a program nib cannot parse is `CannotCheck` naming its type, never `Pass`) — MET for TrueType.
- Oracle 26,361 pairs over 261 documents agree strictly (108 TrueType shapes, 12 declared refusals); corpus 0 false pass
  / 0 false fail over 29,739 pairs. Tiers 4 and 6 did NOT fire: the slice touched `internal/uacheck` only — none of the
  session, ceremony, delivery, discovery, p2p or rendezvous paths.

#### P07.S04 — TrueType per glyph: widths, presence, `.notdef`
Scope: `7.21.5 t1`, `7.21.4.1 t2`, `7.21.8 t1` for simple TrueType and CIDFontType2 (`CIDToGIDMap` stream included) —
`hmtx`, `loca`/`glyf` presence, `post` names, against `pdfops`' dictionary widths through one door.
Acceptance:
- 7.21.5 t1 agrees on its corpus pair; every other half is a veraPDF-measured fixture.
- `len(Clauses())` 104 (the three clauses register here, answering `CannotCheck` for CFF and Type 1 until S05/S06).

#### P07.S05 — the CFF program
Scope: a CFF reader (INDEX, Top DICT, charset, charstring count, widths, CID FDSelect) — `7.21.4.2 t1` (the CharSet
against the program) and the CFF halves of S04's three clauses.
Acceptance:
- 7.21.4.2 t1 agrees on its corpus (2F/1P) and 7.21.8 t1 on its one fail file.
- `len(Clauses())` 105.

#### P07.S06 — the Type 1 program
Scope: eexec and charstring decryption, `/CharStrings` names, `hsbw` widths — the Type 1 halves of 7.21.4.1 t2, 7.21.5
t1, 7.21.8 t1 and 7.21.4.2 t1. **Zero corpus reach**, so every half is a hand-built fixture measured on veraPDF, and
a real-producer Type 1 file is sought first (P08's LibreOffice and Ghostscript output); if none exists the slice may
close as `CannotCheck` naming "Type 1", which the exit criterion already permits — decided at its grill.

### P08 — A real-producer corpus
**Goal.** LibreOffice, Word export, Acrobat and Ghostscript files scored rule by rule, nib against veraPDF.
Refs: D7.
**Exit criteria.** Every rule agrees on the corpus or its disagreement is filed by name; the docs and `nib ua`
say "agrees with veraPDF on N rules", never "conformant" (law 2).

---

## Standing caveats

- **Word and Acrobat output are not producible on this machine.** P08's corpus for those producers needs files
  from somewhere else; LibreOffice and Ghostscript output can be made locally.
- **The census's population is one Markdown fixture.** A loss that only appears on documents nib did not author
  is outside what it can see; P08's corpus is the wider population.
- **Seam inventory:** `~/.claude/projects/-home-dan-repos-nib/memory/instruments/ua-coverage.md`.
