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
- **InsertPDF** is `Collect + Append` — subset and merge, not composition.
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

#### P02.S02 — the n-up carry, repaired
Scope: `tagcarry.go` — visited set by object number, MCR/OBJR `/Pg` repointed and counted, one XObject never bound
to two sources, a non-optimizing read so equal forms stay distinct; a standing identical-pages `NUp` regression.
**`NUp` is routed through S01's completeness predicate HERE**, in the commit that repairs the carry, so the census
row and the both-ways check move together (grill of S01, 2026-09-16). Refs: D5, `/pending 503`'s two tagcarry defects.
Acceptance:
- The census `NUp` row disappears (no 7.20 t2), landing with the carry fix so the both-ways check stays green.
- The four real tagged PDFs keep every element through `NUp`.

#### P02.S03 — redaction does not carry
Scope: `RedactPages` explicitly refuses a structure carry, before any subset carry exists. Refs: the census's
recorded RedactPages decision.
Acceptance:
- A redacted document carries no element from the source tree, measured, with `Collect` carrying.

#### P02.S04 — subset operations carry the tree
Scope: `Collect`/`RemovePages` prune the source tree in place and carry `/MarkInfo`, `/Metadata`,
`/ViewerPreferences` (identification dropped, ADR-032); `DuplicatePage` clones the repeated page under a new key;
refuse a nested page tree or ParentTree. `Booklet`'s rows flip with it; `CarryAttachments`' census rows measure
`Collect(src)`. Refs: D5.
Acceptance:
- `Collect`, `RemovePages`, `DuplicatePage`, `Booklet` census rows add nothing; tag-fate verdicts `carried`.
- A fixture with an OBJR-referenced annotation on a kept and on a dropped page.

#### P02.S05 — crop carries the tree *(blocked — Dan: may a structure tree describe content a crop has clipped from view but not removed?)*
Scope: `Crop` wraps pages in place instead of rebuilding them. Measured compliant even at a 5% window; the
question is what a reader should hear. Refs: D5.

#### P02.S06 — split pages *(blocked — Dan: clone each tile's subtree, so every tile reads the whole page, or drop the claim; either way an ADR)*
Scope: `SplitPage`, `SplitRegions`. Tiles are clones of the page dict carrying the whole content. Refs: D5.

#### P02.S07 — merges graft the second tree *(blocked — Dan: superseding ADR-031's recorded `partial` decision for Append/Combine, which every ceremony document takes)*
Scope: a context-level graft (`pdfcpu.MergeXRefTables`) offsetting the second document's keys and merging root
`/K` and RoleMaps; `InsertPDF` inherits it with S04. Refs: D5, ADR-031.

#### P02.S08 — MCIDs inside a Form XObject are reached through MCR dictionaries
Scope: replace element-level `/Stm` (`tagcarry.go:240`) with MCR dictionaries carrying `/Pg` and `/Stm`, after
reading ISO 32000-1 tables 323–324 and one screen-reader run. Refs: D5.
Acceptance:
- The spec reading is quoted; veraPDF and uacheck still pass the carried `NUp`.

### P03 — Checker: structure-tree containment and roles (~40 rules)
**Goal.** Tables, lists, TOC, headings, notes, Form/Link elements and role maps, each agreeing with veraPDF on
its corpus files. Refs: law 1.

**Exit criteria.** Every rule in the family passes `veracorpus_test.go`; rules without corpus files agree with
veraPDF on fixtures of their own.

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
