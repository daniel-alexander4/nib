# PLAN — PDF/UA coverage: what nib's own edits break, and the checker's other 87 rules

**Dateline.** Seeded 2026-09-15 from `/grill "both option C and tagging"`, approved by Dan the same day.
Every figure below was produced by running veraPDF on 2026-09-15 at v1.129.112, not by reading code.

**Where this plan and the request differ, the plan wins.** The request was *"full tagging of annotations"*
beside option C. Measured, annotations are the smallest part of what nib's own edits break: one operation
breaks annotation rules, while eight page-set operations drop the whole structure tree and three stamping
operations draw in fonts they do not embed. The writing track is built against what was measured.

**Status: P01 closed** (v1.129.119); P02 opened — slices firmed, three blocked on Dan. There is no P00 — nib needs no bootstrap.

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

### P02 — Page-set operations carry the structure
**Goal.** `Collect`, `Crop`, `DuplicatePage`, `Booklet`, `SplitPage`, `SplitRegions`, `InsertPDF` and
`CarryAttachments` keep the structure of the pages they keep. Refs: D5.

**Exit criteria.** Each census row adds nothing, or its remaining loss is declared with the reason a carry is
impossible; the tag-fate table's verdicts move from `dropped` to what is measured.

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

#### P02.S05 — crop carries the tree *(blocked — Dan: may a structure tree describe content a crop has clipped from view but not removed?)*
Scope: `Crop` wraps pages in place instead of rebuilding them. Measured compliant even at a 5% window; the
question is what a reader should hear. Refs: D5.

#### P02.S06 — split pages *(blocked — Dan: clone each tile's subtree, so every tile reads the whole page, or drop the claim; either way an ADR)*
Scope: `SplitPage`, `SplitRegions`. Tiles are clones of the page dict carrying the whole content. Refs: D5.

#### P02.S07 — merges graft the second tree *(blocked — Dan: superseding ADR-031's recorded `partial` decision for Append/Combine, which every ceremony document takes)*
Scope: a context-level graft (`pdfcpu.MergeXRefTables`) offsetting the second document's keys and merging root
`/K` and RoleMaps; ~~`InsertPDF` inherits it with S04~~ **— it does not: S04b routes `splice` through the non-carrying door and defers `InsertPDF` here in full** (struck 2026-09-16). Refs: D5, ADR-031.

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

### P03 — Checker: structure-tree containment and roles (~40 rules)
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
- **THE PHASE'S TRAP, and it is specific to this corpus.** For **twenty** of the family's clauses the corpus
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

#### P03.S01 — role resolution is the spec's algorithm, and the checker owns the standard set
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

#### P03.S02 — the containment matrix, one door
Scope: a declarative table of (standard type → permitted parents, permitted kids) plus one generator that
registers a rule per clause from it. Lands `7.2 t3-t10`, `t17-t20`, `t26`, `t27`, `t36-t38` — seventeen
rules over tables, lists and TOC. Each keeps its own `Clause`, `Summary` and `corpusReach` row; the matrix is
the shared rule, not a shared verdict. Refs: law 1, ADR-009.
Acceptance:
- The matrix is the only place a containment relation is written; a guard asserts every registered
  containment clause routes through it, not that seventeen messages agree.
- **A pass fixture for every one of the fourteen clauses whose corpus is fail-only**, and a red proof that an
  always-Fail implementation is caught by it.
- Every clause's `corpusReach` row measured over the 297-file set.

#### P03.S03 — cardinality and placement
Scope: `7.2 t11-t14`, `t16`, `t28`, `t39`, `t40` — at most one `THead`/`TFoot`/`Caption`, a `TBody` required
when either is present, and `Caption` restricted to first or last kid. Counts and positions, which the
matrix deliberately does not express. Refs: law 1.
Acceptance:
- `t16`, `t28`, `t39` and `t40` have **no corpus file**: each gets a fixture and a recorded veraPDF verdict.
- An element with no kids is `NotApplicable`, never a silent Pass.

#### P03.S04 — table geometry: spans, and the grid nib does not reproduce
Scope: `7.2 t15` (cells shall not intersect), `t41`, `t42`, `t43` (rows and columns agree once spans are
counted). The hardest four, and the one place the checker already **declares** a gap —
`rules_semantic.go:182` returns `CannotCheck` for a grid it does not build. Either the grid arrives here and
that `CannotCheck` retires, or it stays and this slice says which rules it costs. Refs: law 1, law 4.
Acceptance:
- `t42` and `t43` land together — identical wording, opposite polarity.
- Where the grid cannot be built the verdict is `CannotCheck` naming why, never `Pass`; the existing
  `reach_test.go` shape covers it.

#### P03.S05 — strongly or weakly structured, but not both
Scope: `7.4.4 t1` (at most one child `H` per node), `t2` and `t3` (a document uses `H` or `Hn`, never both).
Interacts with the implemented `7.4.2 t1`, whose `corpusReach` is 134 — the widest in the family — so a
change to heading handling is measurable immediately. Refs: law 1.
Acceptance:
- A document using both `H` and `Hn` fails t2 and t3; one using neither is `NotApplicable`.
- `7.4.2 t1`'s `corpusReach` of 134 does not move, or the change is explained in the same edit.

#### P03.S06 — notes, the Form element, and the parent entry
Scope: the residue — `7.9 t1` and `t2` (`Note` has an `ID`, and IDs are unique), `7.18.4 t2` (a `Form`
omitting `Role` has exactly one object-reference child), `7.1 t12` (every element carries `/P`), `7.5 t2`
(the sibling of the implemented `t1`). Refs: law 1.
Acceptance:
- `7.1 t12` and `7.18.4 t2` have no corpus file; both get a fixture and a recorded veraPDF verdict.
- `7.9 t2`'s uniqueness is checked across the whole tree, not per subtree.

### P04 — Checker: language (~10 rules)
**Goal.** Outline entries, ActualText/Alt/E, annotation Contents, form TU and marked-content spans.
**Exit criteria.** As P03.

### P05 — Checker: annotations (~9 rules)
**Goal.** Annotation containment, alternate descriptions, tab order, links, media clips, TrapNet, PrinterMark.
**Exit criteria.** As P03.

### P06 — Checker: file-level rules (~11 rules)
**Goal.** Identification prefix and properties, header, Suspects, embedded-file keys, XFA, encryption P,
reference and Form XObjects, Formula.
**Exit criteria.** As P03.

### P07 — Checker: font programs (~12 rules)
**Goal.** CMaps, CIDToGIDMap, width agreement, TrueType cmap and encoding, CharSet, `.notdef`, ToUnicode values.
**Exit criteria.** As P03, and a font program nib cannot parse returns `CannotCheck` naming its type, never
`Pass`. Refs: law 1. Residual doubt: `/pending 493`.

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
