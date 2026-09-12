# PLAN — structure tagging and accessibility, to Acrobat parity

**Dateline.** Seeded 2026-09-06 from `/grill "coming to feature parity with Adobe Acrobat for
structure tagging and accessibility"`, whose measured findings are this plan's factual base and
whose scoped counter-proposal Dan **overruled in favour of full parity** (option B, 2026-09-06).

**Where this plan and the original brief differ, the plan wins.** Where this plan and a memory
entry differ, the plan wins — `/pending 29`'s body predates every measurement below and three of
its claims are corrected here.

**Status: unbuilt.** No slice has started. `/createcode` drives it from P01.

---

## What this is

Adobe Acrobat's accessibility suite is four products in a trench coat: a **heuristic autotagger**
for documents it did not create, a **structure editor** (Tags panel, Reading Order tool), an
**in-app conformance checker** with a remediation report, and a **batch action** that runs the
first three over a folder. Parity means all four, plus the tagged output itself, plus an
application a keyboard-only user can actually operate.

**There is no P00.** PLANNING.md reserves it for the bootstrap commit; nib is eleven hundred
commits old and needs no scaffolding. Product work starts at P01.

## The measured starting point

Every number here was produced by running something on 2026-09-06, not by reading code. They are
restated in the plan because a decision that cites a measurement should carry it.

- **veraPDF 1.30.2** is installed, supports flavours `ua1` and `ua2`, and costs **2.32 s** per
  file. It is Java; nib is cgo-free. It can be a test oracle and can never be the in-app checker.
- A document nib authored end to end (`nib office` on markdown → its own `mdpdf` emitter →
  `nib pagenum`) fails PDF/UA-1 on **9 distinct rules, 33 checks**. Only two rules are the
  structural half; the rest is catalog plumbing (`/MarkInfo`, `/Metadata`, `DisplayDocTitle`, two
  optional-content rules), `/Lang`, and font embedding.
- **Nib destroys tagging that arrives in the door.** `pages`, `split`, `redact`, flatten, and
  `merge`-when-the-tagged-file-is-not-first drop `/StructTreeRoot` and `/MarkInfo` outright.
- **`nup` is worse than a strip: it lies.** It keeps the tree, `/ParentTree`, `/RoleMap` and
  `/MarkInfo /Marked true` while destroying every `/StructParents`, BDC/EMC pair and MCID, leaves
  struct elements pointing at pages that no longer exist, and still validates under pdfcpu.
  veraPDF reports clause 7.1 t3 as a **new** failure the same file passed before `nup` touched it.
- **`/Lang` reaches one door.** `pdfops.SetLang` has exactly one production caller —
  `internal/server/ocr.go:86` — so only OCR'd documents carry a language. That is rule 7.2 t34 and
  8 of the 33 failures. `lang_test.go` guards that the page primitive *preserves* `/Lang`, not that
  documents *receive* one.
- **Nib hand-emits zero PDF content-stream operators.** Every text path — OCR layer, watermark,
  page numbers, form fields, `mdpdf` — goes through `pdfcpu/pkg/api`. Emitting marked content is a
  new capability, not an extension of one.
- **pdfcpu v0.13.0 round-trips a hand-built tag tree byte-identically** (187 → 187 bytes, no MCID
  renumber) and has no tagging builder: its only `StructTreeRoot` code is validation, one
  write-out, and two deletions.
- **The structure nib needs already exists in two places and is thrown away.** `mdpdf` walks a real
  AST (`ast.Heading` with a level, `ast.Paragraph`, lists — `mdpdf/mdpdf.go:171-180`). The vendored
  tesseract.js 5.1.1 returns `block`, `paragraph` and `line` references on every word in reading
  order; `web/app.js:6334` copies out the text and box and drops the rest.
- **Nib's own UI fails WCAG 2.1 AA at SC 2.1.1 today.** Every annotation creation path is
  `pointerdown`-only and there is no arrow-key nudge (`grep ArrowUp web/app.js` → 0). `aria-pressed`
  appears nowhere in first-party code while ~15 tools signal armed state with a CSS class alone.

## Repo laws this plan establishes

These bind every slice below and every change made after this plan retires. Each becomes an ADR.

1. **Nothing claims tagging it does not have.** No output may carry `/MarkInfo /Marked true`, a
   `/StructTreeRoot`, or a conformance assertion over content that is neither tagged nor marked as
   an artifact. A visible loss is honest; a false claim is not, and it is worse than no tagging at
   all because it defeats the reader's own check.
2. **Every operation declares its tag fate.** `carried` / `refused` / `dropped-with-notice` — one
   verdict per operation, in one table, and the guard asserts that every operation *has* one. This
   is ADR-009's shape: the guard checks the door, not the eight sites that happen to be right.
3. **Inferred structure is a proposal, never an assertion.** Structure nib *observed* (its own AST,
   its own form fields, tesseract's blocks) may be written silently. Structure nib *guessed* from an
   arbitrary PDF is presented for review before it is written, the way **Detect** already proposes
   form fields. A wrong tag tree misleads a screen reader with more authority than no tag tree.
4. **The checker never reports a pass it did not perform.** Three verdicts — pass, fail, and
   *cannot check, and why* — and the third never collapses into the first.
5. **The oracle validates the checker.** nib's pure-Go checker is itself checked against veraPDF
   over a golden corpus. A checker nothing checks is the fatal-bug category of this whole plan.

---

## Decisions

### D1 — The target is PDF/UA-1, and veraPDF `ua1` is the test oracle *(settled 2026-09-06 via /grill)*
ISO 14289-1 is what procurement asks for and what the installed oracle validates. UA-2 is out of
scope for this plan and gets its own item when a real requirement appears. The oracle enters the
tree as one word changed in the existing PDF/A helper — `requireVeraPDFCompliant(t, pdf, "ua1")` —
and skips cleanly when veraPDF is absent, recorded as `not exercised`, never credited as a pass.

### D2 — Preservation precedes authoring *(settled 2026-09-06 via /grill)*
No phase adds tagging while an operation silently destroys it. This inverts the order the brief
implied and is the plan's single most load-bearing sequencing decision: tagging authored on top of
a pipeline that eats tagging produces documents that are accessible until the user rotates a page.

### D3 — Emission wraps, it never re-emits *(settled 2026-09-06 via /grill)*
Marked content is produced by **wrapping** what pdfcpu already emits — `BDC` before, `EMC` after,
around an existing text run or an existing Form XObject `Do` — never by hand-emitting `Tj` and
re-encoding glyphs. This is why the CID work `/pending 29` budgeted for does not appear in this
plan: the OCR path already makes one Form XObject per word, and wrapping a `Do` touches no font
encoding at all. Confirmed by probe: pdfcpu passes content-stream bytes through verbatim, so a
wrap survives the write path.

### D4 — Structure has three sources, ranked by fidelity, and the rank is visible to the user *(settled 2026-09-06 via /grill)*
**Observed-exact** (`mdpdf`'s AST, the form fields nib itself placed) → written silently.
**Observed-approximate** (tesseract's block/paragraph/line) → written silently, marked in the tag
tree as OCR-derived. **Inferred** (the autotagger over an arbitrary PDF) → proposed for review per
law 3. The user is told which of the three produced the tree they are looking at.

### D5 — The autotagger follows Detect's interaction model *(settled 2026-09-06 via /grill)*
Nib already has a heuristic proposer whose output the user edits before it becomes real: **Detect**
for form fields. The autotagger is the same shape — propose, show, let the user move/retype/ignore,
then commit — and reusing that model is worth more than any tagging-specific UI invention.

### D6 — Nib ships its own pure-Go checker; veraPDF stays behind the test line *(settled 2026-09-06 via /grill)*
Measured: veraPDF is a 2.32 s JVM invocation, and nib is a single cgo-free binary with no runtime
dependencies. An in-app checker must therefore be nib's own code. It does not need to implement all
of PDF/UA — it needs to implement what it can verify and to say plainly what it cannot (law 4),
with agreement against veraPDF asserted over a corpus (law 5).

### D7 — Authored text embeds its fonts *(settled 2026-09-06 via /grill)*
Rule 7.21.4.1 fails on nib's own output because `mdpdf` draws in Base-14 core fonts
(`mdpdf/mdpdf.go:37-40`) and `coreFont()` falls back to Helvetica. `markdownFallbackFonts()` already
supplies embeddable faces and pdfcpu's user-font path subsets and writes `/ToUnicode`, so the
machinery exists. Authored output embeds; core-font *metrics* stay for layout. Where a document
nib did not author carries non-embedded fonts, the existing `nonEmbeddedFonts()` sweep becomes a UA
blocker and the export **refuses**, mirroring `pdfaBlockers`' refuse-rather-than-mislabel idiom.

### D8 — The tag tree is a typed Go model, not dictionary manipulation at call sites *(settled 2026-09-06 via /grill)*
pdfcpu has no builder, so nib gets one: a typed tree that parses an existing `/StructTreeRoot`,
mutates it, and writes it back, with `/ParentTree`, `/StructParents` and MCIDs maintained as
invariants of the model rather than by each caller. Every operation in law 2 that reports `carried`
routes through this one model.

### D9 — The page-subset remap is a measured prerequisite, not an assumption *(settled 2026-09-11 by P01.S02's SECOND measurement; the first is struck below and kept)*

**Read the correction before the decision.** This section has been wrong twice and both are recorded,
because the second error was made *while correcting the first* and that is the more useful lesson.

~~`Collect`, `RemovePages` and `SplitBySpans` drop the tree today. Carrying it means remapping
`/ParentTree` and `/StructParents` for a kept subset and pruning orphaned elements. The grill
measured **preservation in place** and explicitly did **not** measure a remap. P01.S02 measures it
before anything is planned on top of it.~~

~~**The answer, and it is one level below the question D9 asks.** There is no subset to remap, because
**the write path carries no struct element at all.** A no-op `writeMutated` loses every
`/StructElem`; `Rotate` keeps the claim over zero elements and is a live law-1 violation; every
`writeMutated` caller is a candidate. P05's tag-tree core cannot be built until the write path
carries a tree.~~

**(STRUCK 2026-09-11. Every figure above came from `bytes.Count(pdf, []byte("/StructElem"))`, and
pdfcpu writes the structure tree into a compressed object stream — so that count is `0` for every
pdfcpu output whatever it contains. The write path carries trees perfectly well. The enforcement
built on this reading stripped trees that had survived; see ADR-031.)**

**The measurement, parsed.** A LibreOffice-produced tagged PDF — 4 pages, `/MarkInfo /Marked true`,
**45 `/StructElem`**, every page carrying `/StructParents` — through pdfcpu v0.13.0. *Anchored*
counts elements whose `/Pg` is a page still in the page tree; *undescribed* counts pages that have a
content stream and no `/StructParents`:

| operation | claim | elements | anchored | undescribed | fate |
|---|---|---|---|---|---|
| **source** | yes | 45 | 45 | 0 | — |
| no-op `writeMutated` | yes | 45 | 45 | 0 | **carried** |
| `Rotate`, `Optimize`, `SetLang`, every stamp and strip (19 in all) | yes | 45 | 45 | 0 | **carried** |
| `Collect`, `RemovePages`, `Crop`, `SplitPage`, `DuplicatePage`, `InsertPDF`, `Booklet` | no | 0 | — | — | **dropped**, honestly |
| `Append(tagged, untagged)` | yes | 45 | 45 | **1** | **partial** |
| `NUp(2)` | yes | 45 | **0** | 2 | **ORPHANED — law 1's violation** |

**Three findings, in order of how much they cost.**

1. **26 of the 48 operations carry the tree**, and every one of them was declared `dropped`. The
   write path is sound; `/pending 29`'s reason 1 stands as originally measured on 2026-06-23.

2. **`NUp` is a genuine, shipped law-1 violation and it was invisible to the corrected oracle too.**
   The replacement predicate was *"claims tagging and has zero struct elements"*, and `NUp` emits
   **45** — all pointing at page objects `api.NUp` left behind when it composed its sheets, with no
   composed sheet carrying `/StructParents`. veraPDF ua1 agrees independently and in law 1's own
   words: source and `Rotate` fail 5 t1 / 7.1 t9 / 7.1 t10 alike, while the n-up output adds
   **7.1 t3, *"Content shall be marked as Artifact or tagged as real content"*, 24 failed checks.**
   So P01.S01's heading was right and its evidence was wrong — an unusual combination, and the
   reason the fix is reinstated rather than re-argued.

3. **D9's actual question is answered for the operations that drop, and stays open for a remap.**
   `Collect`, `RemovePages` and `SplitBySpans` drop claim and content together, which is law 1
   satisfied honestly and needs no notice mechanism to be compliant. Whether `/ParentTree` +
   `/StructParents` *could* be remapped to carry a kept subset is still unmeasured and still worth
   measuring — it is now an improvement rather than a prerequisite, and it is P03's.

**Why `partial` is recorded and not enforced — settled 2026-09-11 by `/pending 468`.** `Append`
keeps a live 45-element tree and leaves one appended page undescribed. Stripping the claim would
destroy the whole tree to fix one page, and `p2p/readme.go` and `p2p/sigpages.go` take this path for
**every ceremony document** — so the cure is worse than the disease at the only scale that matters.

**The middle way was proposed and is REFUTED by measurement.** The proposal was to drop only
`/MarkInfo /Marked true` and keep the tree, on a reading that `/StructTreeRoot` alone asserts nothing.
Measured with veraPDF ua1 on the same partial document: the edit **keeps** 7.1 t3 and 7.21.4.1 t1 and
**adds** 6.2 t1. It cannot work and the reason is structural — **7.1 t3 is about the content stream,
not the catalog**, so no catalog edit can satisfy it. The same fact was already in hand from
P01.S01's measurement, where stripping *both* keys from the n-up output left 7.1 t3 in place.

**And the same probe found the state nothing could see.** `api.MergeRaw` merges two TAGGED documents
by keeping the first document's `/StructTreeRoot` and `/ParentTree` whole while every page of the
second keeps its own `/StructParents` — measured: an 8-page merge carries a **4-entry** `/ParentTree`
and eight pages indexing keys 0–3. The second document's pages therefore have a `/StructParents` that
resolves to structure describing different content, no element points at them, and a reader walking
the tree from the root never reaches them. **The census rated that `carried`**, its best verdict,
because `undescribed` asked whether a page HAS a `/StructParents` rather than whether anything points
AT it. The predicate is now reachability, which catches both shapes with one question, and
`Combine` — the package's other `MergeRaw` caller, invisible to the enumeration until it stopped
requiring a `[]byte` first parameter — carries a verdict for the first time.

**So the disposition is: record, do not enforce, and make the recording true.** The honest fix is to
mark the appended pages' content as artifacts or merge the two `/ParentTree`s, both of which are
authoring and P05's. `Append(tagged, tagged)` is the one case where the data to do it exists — the
second tree is built and then discarded.

**Measured, because a remedy is a claim too:** stripping the n-up output's claim does **not** remove
7.1 t3 (the content is untagged either way) and adds 6.2 t1 and 7.1 t11, since PDF/UA requires a
structure tree. A veraPDF failure *count* therefore scores honesty as a regression — which is why
P01.S01's acceptance is a structural property and not a clause count, and that strike stands.


### D10 — The editing surface lives in the Document tab *(settled 2026-09-06 via /grill)*
Per ADR-016, a mode is a kind of thing you do to the document. A tag tree changes the document
itself rather than adding something on top of the page, so the Tags panel and Reading Order view
are **Document**, not Mark Up. The panel is a sidebar accordion card per ADR-018 and ADR-020.

### D11 — The application's own accessibility is in scope and is not last *(settled 2026-09-06 via /grill)*
Acrobat's accessibility story includes an operable application. Nib's fails SC 2.1.1 today: no
annotation can be created without a pointer. Parity that ships PDF/UA output from an application a
keyboard-only user cannot draw in is not parity, so P02 sits early rather than at the end.

### D12 — The corpus is guard-test law *(settled 2026-09-06 via /grill)*
This plan's fatal-bug category is *"claims accessible, is not"*. Its correctness oracle is a golden
corpus: tagged and untagged fixtures, one per structure kind and one per known-destructive
operation, with expected verdicts. PLANNING.md's failure mode 6 — the fatal-bug category with no
oracle — is the reason this is a decision and not a test-plan footnote.

---

## Build order

### P01 — Preservation and honesty
**Goal.** Stop nib degrading documents that arrive tagged, and stop it claiming tagging it does not
have. Nothing in this phase authors any structure; it is the floor D2 requires, and it is
independently worth shipping even if the plan went no further.

**Exit criteria.**
- No operation emits a tagging claim over unmarked content; `nup`'s output no longer regresses
  veraPDF clause 7.1 t3 against a tagged input.
- Every operation that touches a document has a declared tag verdict, and a guard fails when a new
  one has none.
- The tag-fate table and its law ship as an ADR.

#### P01.S01 — `nup` stops lying *(done 2026-09-11, v1.129.16 — shipped at v1.129.9 on a wrong measurement, reverted at v1.129.15, REINSTATED on the right one)*
Scope: `nup` either drops `/StructTreeRoot` and `/MarkInfo` with the content it voids, or tags its
composed page; it may not keep the claim. Refs: law 1, D2.
Acceptance:
- ✅ A tagged input through `nup` produces no veraPDF failure the input did not already have.
  ~~**(struck 2026-09-11 — only the option this slice defers could have met it.)**~~
  **(UN-STRUCK 2026-09-11 at the phase close, and this is the more interesting correction of the
  two.)** The strike was right about *dropping*: stripping the claim cannot remove 7.1 t3, because
  t3 is about the content, and it adds 6.2 t1 and 7.1 t11 on top. It was wrong that dropping was
  the only option — the content survives the composition intact inside Form XObjects, so the tree
  can be re-anchored. **P01.S06 does that and the clause is met as originally written.** The
  clause was struck because the remedy in hand could not reach it; the remedy was the limit, not
  the clause.
- ✅ **Replacing it:** the output carries no `/MarkInfo /Marked true` and no `/StructTreeRoot` —
  law 1 as a structural property of the output, which is what it actually says.
- ✅ The output carries no struct element whose `/Pg` points at a page that is not in the document.
  This is now `tagState.anchored`, and it is the clause that survived both measurements intact.
- ✅ A red proof: `TestNUpDoesNotClaimTaggingItHasNot` goes red against `return out.Bytes(), nil`,
  which is the shipped v1.129.4 behaviour verbatim.

**(pin, 2026-09-11 — this slice was right, then withdrawn as wrong, then reinstated. All three states
are recorded because the middle one is the instructive part.)**

The founding premise was `/pending 29`'s reason 1 — that pdfcpu round-trips a tagged document intact
— and the slice reported it REFUTED on the strength of `bytes.Count(pdf, []byte("/StructElem"))`.
pdfcpu writes the tree into a compressed object stream, so that count is `0` for every pdfcpu output.
**Reason 1 was never refuted; it stands as measured on 2026-06-23.** The whole slice was reverted at
v1.129.15 along with the enforcement, which had been stripping trees that survived.

**But `nup` was lying, and it still is at the released v1.129.4.** Parsed: the n-up output keeps
`/Marked true` and a 45-element `/StructTreeRoot` whose every element points at a page object that is
no longer in the page tree, with no composed sheet carrying `/StructParents`. veraPDF ua1 scores it
7.1 t3 — *"Content shall be marked as Artifact or tagged as real content"* — with 24 failed checks,
against an input that passes that clause. Three oracles, two of them independent of this repo.

**What changed in the fix, and it is the whole difference between v1.129.9 and v1.129.16:** the drop
is a POST-CONDITION (`honest`), asked of the output and acted on only when the tree anchors to
nothing. v1.129.9 stripped unconditionally. `TestHonestLeavesACarriedTreeALONE` is the guard for
that distinction and goes red — byte-for-byte, on `Rotate` and `Optimize` output — against an
unconditional strip.


#### P01.S02 — measure the page-subset remap *(done 2026-09-11, v1.129.16 — re-run with a parsing oracle after the first run's was refuted)*
Scope: probe whether `/ParentTree` + `/StructParents` can be correctly remapped for a kept page
subset against pdfcpu v0.13.0. Outcome, not code, is the deliverable; it settles D9. Refs: D9.
Acceptance:
- ✅ A recorded measurement, not an argument. **31 operations driven against a producer-made tagged
  document and against the committed fixture, and the two agree operation for operation.** The table
  is at D9.
- ✅ D9 is settled with the answer, and it is the opposite of the first run's: the write path
  carries trees, 19 operations preserve them, and the page-set operations drop claim and content
  together — law 1 satisfied honestly, needing no notice to be compliant.
- ✅ **The fixture is a real LibreOffice-produced tagged PDF**, and that was the point of running it.
  45 struct elements from a producer.
- ✅ **A remap is no longer a prerequisite.** It would be an improvement — carrying a tree across a
  kept subset rather than dropping it — and it moves to P03 as such.

**(pin, 2026-09-11 — this slice was marked done, reopened the same day, and closed again. The reopen
is why the answer is trustworthy.)** The first run used `bytes.Count(pdf, []byte("/StructElem"))`
and concluded the write path carries nothing. **The discriminator this slice's own item demanded —
a known-good producer-made document — WAS run, and read with the same broken instrument, so it
confirmed the error.** A discriminator is only as good as the instrument reading it. The second run
parses, and is corroborated by veraPDF, which is outside this repo entirely.


#### P01.S03 — the tag-fate table and its guard *(done 2026-09-11, v1.129.16 — the CENSUS stood throughout; 27 of its 48 VERDICTS were wrong and are now measured)*
Scope: every document-touching operation declares `carried` / `refused` / `dropped-with-notice` in
one table; a table-driven tier-1 guard over a tagged fixture asserts each verdict and fails when an
operation has no entry. Refs: law 2, D12.
Acceptance:
- ✅ The guard enumerates operations from the code, not from a hand-written list. `go/ast` over
  every exported function in `internal/pdfops` whose first parameter is a `[]byte` and whose first
  result is a document — **49 operations**, with a floor so an enumeration that stopped matching fails
  loudly instead of passing empty. Matching on the SHAPE rather than the parameter NAME is what
  found `RedactPages` and four others.
- ✅ Adding an operation with no verdict turns it red, **proved by adding one** (`StampImages`,
  genuinely missed; red, then driven).
- ✅ **Every declared verdict is the MEASURED verdict**, asserted per operation —
  `TestEveryDeclaredFateIsTheMEASUREDFate`, 29 of 34 driven with zero skips. Probed red against the
  two verdicts history actually got wrong: `Rotate` as `dropped` and `Append` as `dropped`.
- ✅ The fixture corpus lands under D12's corpus — `internal/pdfops/corpus_test.go`, shared by every
  tag guard, rather than inline in one test. **Generated rather than committed**, for the reason
  this package's own fixture file already gives: *"a checked-in binary fixture is opaque in review"*.

**The verdict vocabulary, and it took three attempts to get the words right.** The scope line said
`carried` / `refused` / `dropped-with-notice`. The first build shipped `dropped` / `carried` /
`untouched` and declared **`dropped` for all 33**, on a byte count. The vocabulary is now
**`carried` / `dropped` / `partial` / `untouched`**, and `partial` is the word that was missing:
a live tree that does not reach every page with content. `refused` never appeared because no
operation refuses a tagged input.

**`orphaned` is still not a verdict an operation may declare** — it is law 1's violation, and the
guard fails it rather than recording it. That much of the original reasoning was right.

**(pin, 2026-09-11 — the census was sound and every verdict in it was false, which is a failure mode
worth naming.)** The table shipped as 39 rows of `dropped` and 9 of `untouched`, the first
derived from a byte count that cannot see a compressed object stream. **26 of them carry the tree
intact and one is `partial`.** Nothing caught it because the guard
only ever asked *"does this output lie?"* under the weakest available definition of lying — it never
compared a DECLARATION to a document. A census that cannot be wrong is not a census, and the
correctness half (`TestEveryDeclaredFateIsTheMEASUREDFate`) is the missing assertion.

**And the weak definition hid the one real violation.** *"Claims tagging and has zero struct
elements"* cannot see `NUp`, which emits 45 elements pointing at pages that are gone. The predicate
written to enforce law 1 was blind to the only operation in the package that breaks it.


#### P01.S04 — carry the tree where it can be carried *(done 2026-09-11, v1.129.16)*

**(pin, 2026-09-11 — marked `done` on a premise that was the exact inverse of the truth.)** The
marker first went on because *"D9 measured that nothing can be carried, so every operation is
`dropped`"*. **19 operations carry the tree**, so this slice's carrying half was not vacuous — it
was already satisfied, by pdfcpu, and the plan had no idea. The clause below that was struck for
being impossible turns out to describe a live defect. Both are corrected in place.

Scope: implement carrying for the operations S02 found practical; the rest become
`dropped-with-notice` with a user-visible sentence. Includes `merge`'s argument-order defect —
tagging survives only when the tagged file is first. Refs: D8, D9, law 2.
Acceptance:
- ✅ `merge` is asserted per argument order, and **the defect this clause names is REAL** —
  `api.MergeRaw` takes the first document's catalog whole, so tagged-first keeps the claim and the
  whole 45-element tree, and tagged-second drops both.
  ~~**Overtaken by D9: nothing preserves tagging in any order, so this defect cannot exist.**~~
  **(struck 2026-09-11 — that reading came from a byte count. The clause was right; it was struck
  on evidence that could not see a compressed object stream.)** What is asserted is the measured
  fate per order: tagged-first is `partial`, tagged-second is `dropped`, and **neither is
  `orphaned`**. Preservation in both orders is not achievable through `MergeRaw` and is not this
  slice's to build; the honesty property is, and it holds.
- ✅ **`partial` is a recorded verdict rather than an enforced one, and that is a decision.**
  `Append(tagged, untagged)` leaves one appended page undescribed under a live tree. Stripping would
  destroy 45 good elements to fix one page, and `p2p/readme.go` and `p2p/sigpages.go` take this path
  for **every ceremony document**. Parked for Dan: whether law 1 should be superseded to permit a
  `/StructTreeRoot` over a partially-described document, or whether P05 should artifact the appended
  pages instead.
- ✅ **Every `dropped-with-notice` operation actually emits its notice, asserted at the door.**
  Dan chose **a persistent banner modelled on `#fitNotice`**, 2026-09-11, over a toast — on the
  repo's own recorded reasoning about what a self-clearing toast can carry (`index.html:72`:
  *"`toast` cannot carry them: it clears itself after 2500 ms"*). A user who lost their document's
  accessibility structure has to still be able to see it **at the moment they save**.

  **Asserted at the COMMIT door, not at 48 operations.** `noteTaggingFate` runs from
  `commitMutation` and `commitBarrier` — the only two places a mutation's result becomes the
  document, and the only two that hold both the before and the after. Asking each operation to
  report its own fate would be ADR-009's rule inverted: 33 sites that have to remember, against two
  that cannot forget.

  **Sticky per document, not per operation**: six edits later the tagging is still gone, and a flag
  that cleared on the next commit would be gone before it mattered.
- ✅ **`redact` is explicitly dispositioned** — and discharging it exposed a hole in the census
  itself. `RedactPages`
  (`internal/pdfops/pdfops.go:354`) does not match S03's enumeration shape — its first parameter is
  `original`, not `pdf`, and it takes a raster map — so the guard never saw it, which was a **hole in
  the enumeration** rather than an operation that is exempt. The enumeration now matches on the
  SHAPE, and `RedactPages` carries an explicit verdict with its reasoning: a
  redaction that kept a tag tree would let a reader recover the shape of what was removed.

#### P01.S05 — the ADR and the corpus *(done 2026-09-11, v1.129.12)*
Scope: ADR for laws 1 and 2; the golden corpus per D12 with its expected verdicts. Refs: D12.
Acceptance: the ADR names the operations, the corpus is loaded by S03's guard, and the ADR is cited
from the tag-fate table.
- ✅ **ADR-031** names all eight operations the guard caught, carries the measurement rather than
  the argument, and records why the check is a post-condition that expires on its own.
- ✅ The corpus is `internal/pdfops/corpus_test.go` and is loaded by S03's guard, S01's guards and
  S04's merge test — one definition of "tagged", shared, rather than each guard inventing its own.
- ✅ Cited from the tag-fate door (`internal/pdfops/tagfate.go`) and indexed in `docs/adr/_index.md`,
  which `TestSupersededADRsSaySoAndEveryADRIsIndexed` enforces.
- **The ADR records two things the plan did not ask for, because both are the instructive part**:
  that the enumeration's first filter was itself a hole (it matched on a parameter NAME), and that
  veraPDF's conformance score cannot express law 1 — dropping a claim *adds* UA-1 failures, so a
  failure count scores honesty as a regression.

#### P01.S06 — carry the tag tree through `nup` *(done 2026-09-11, v1.129.19)*

**(added 2026-09-11 at the phase close, because the phase's own exit criterion was unmet and the
reason it was unmet was not the reason anyone assumed.)** P01.S01 made `nup` honest by dropping the
claim, and the phase's exit criterion asks for something stronger: *"`nup`'s output no longer
regresses veraPDF clause 7.1 t3 against a tagged input."* Measured on the shipped v1.129.18 output —
it still does, along with 6.2 t1 and 7.1 t11 that dropping the claim itself introduces.

**The content was never destroyed.** veraPDF's failing contexts read
`xObject[0]/contentStream[0]/content[2]{mcid:0}` — the original page content, still inside its
`BDC …EMC` marked-content sequences with its MCIDs intact, now living in a Form XObject. Measured on
the raw `api.NUp` output of a 4-page tagged document: 2 sheets, 4 Form XObjects whose decoded content
is **byte-identical** to the four original pages, 45 struct elements still pointing at the four
original page objects, and a `/ParentTree` that still holds all four entries keyed 0–3. What is
missing is anchoring, and only anchoring: no XObject carries `/StructParents`, and every element's
`/Pg` names a page that is no longer in the page tree.

Scope: remap the surviving tree onto the composed sheets — give each Form XObject the
`/StructParents` its source page had, and repoint each element's `/Pg` at the sheet it now appears
on with `/Stm` naming the XObject (PDF 32000-1 §14.7.4.4). **Nothing is authored**, which is what
keeps this inside P01's goal and on D2's side of the preservation/authoring line. Refs: law 1, D2,
D9, P01 exit criterion 2.

Acceptance:
- ✅ A tagged input through `nup` adds **no veraPDF ua1 clause the input did not already fail** —
  7.1 t3 in particular. Measured three ways: the 4-page LibreOffice fixture now fails exactly its
  input's three clauses (5 t1, 7.1 t9, 7.1 t10) where before the carry it added 7.1 t3, 6.2 t1 and
  7.1 t11; **`boi.pdf` fails exactly what its source fails**; and **`adgm_va.pdf` fails a strict
  SUBSET** of its source's ten clauses. The last two are real producer documents, not fixtures.
- ✅ Every struct element's `/Pg` is a page in the document, and no page with content is
  unreachable from the tree. Enforced in the carry itself — an element whose `/Pg` names a page it
  cannot place is `stranded` and abandons the whole remap — and asserted as `undescribed == 0`.
  **Measured on four real tagged PDFs: 3800, 2247, 1157 and 128 elements, every one preserved**,
  `undescribed` 0 in all four. (`anchored` runs 3 to 15 short of `elements` on each, which is
  correct: a `/Document` or `/Sect` container carries no `/Pg` of its own.)
- ✅ **A remap that cannot anchor everything drops the claim instead of shipping a partial one.**
  Probed red: removing both all-or-nothing guards makes `TestTheCarryIsABANDONEDRatherThanShipped``HalfDone` report success over a document containing none of the XObjects it re-anchors.
- ✅ An untagged input is returned by the new path **unmodified** — `inspectTags(raw).orphaned()`
  is false and `raw` is returned as-is, so it costs one parse and no rewrite.
  ~~byte-identical~~ **(clause reworded 2026-09-11 — byte identity was asserted across two separate
  compositions and that cannot hold: pdfcpu writes a fresh `/ID` and `/ModDate` per run, so two
  n-ups of one input differ by construction. Measured: same length, different bytes.)** The
  verifiable claim is that the carry DECLINES, which is asserted directly.
- ✅ `Booklet` is unaffected and stays `dropped`: it composes `InsertBlank` → `Collect` → `NUp`, and
  `Collect` has already dropped the tree before `NUp` sees it. Held by the census, which measures
  every declared verdict.

**Cost, measured rather than reasoned.** On a 1.4 MB 22-page document with 3800 elements:
`api.NUp` alone 147 ms, the orphan check 88 ms, the whole shipped path 559 ms — the carry costs
~324 ms and buys 3800 preserved elements. **The check order is what keeps that off the common
case**: asking the OUTPUT whether it is orphaned comes first, so a document with nothing to carry
pays one parse and stops.

**Not dived**: the slice touches `internal/pdfops` only and authors no seam — no wire format, no
stored layout, no interface with several implementors. **Slice gate does not fire**: nothing in
`internal/server`'s session/ceremony/delivery/discovery, `internal/p2p` or `internal/rendezvous` is
touched.

### P02 — The application's own WCAG 2.1 AA
**Goal.** Make nib operable without a pointer and legible to a screen reader. This is a live
conformance failure in shipped code (D11), and it is independent of every PDF concern below.

**Exit criteria.**
- Every annotation tool can create, move and resize a mark by keyboard alone, asserted at tier 3.
- Toggle state is exposed programmatically, not by colour alone, across every armed tool.
- A keyboard-only pass over the primary flows — open, mark up, save — completes with no trap and no
  stranded focus.

~~Sketched slices: keyboard creation and arrow-nudge for every tool · `aria-pressed` across the
armed-tool set and `aria-expanded` on panel accordion cards · the toast live region announcing its
first message · accessible names for the 21 icon-only buttons currently named by `title` alone ·
`prefers-reduced-motion` and a non-text contrast guard (SC 1.4.11) · a 200%-zoom reflow assertion.~~

**(phase-open, 2026-09-11 — the sketch's baselines are re-measured against the tree as it now is,
and three of the six have moved. One of them this session falsified itself.)**

| sketched | measured 2026-09-11 | verdict |
|---|---|---|
| *"`grep ArrowUp web/app.js` → 0"* | **0** | **holds** — no keyboard creation or nudge exists |
| *"`aria-pressed` appears nowhere in first-party code"* | **10 occurrences** (5 `index.html`, 5 `app.js`) | **false, and falsified by this session**: the View group added at v1.129.4–.6 uses it. What survives is the ratio — **27 `classList.toggle('active')` against 4 `setAttribute('aria-pressed')`** — so the defect is real and the sentence naming it is not |
| *"accessible names for the 21 icon-only buttons named by `title` alone"* | **4** buttons have no visible text and no `aria-label` (`themeToggle`, `updateGet`, `sessionNoticeAction`, `signAction`); 3 already have one | **badly stale** — the count is off by a factor of five, and a slice scoped to 21 would have spent most of its effort discovering there was nothing to do |
| *"the toast live region announcing its first message"* | the toast **already carries `role="status"` + `aria-live="polite"`**, set at creation, since `/pending 328` | **built** — what may survive is the first-message nuance (a live region created in the same tick as its first message is not announced), which is a different and much smaller slice |
| `prefers-reduced-motion` / SC 1.4.11 | not measured here | unchanged |
| 200%-zoom reflow | not measured here | unchanged |

**Firmed slices:**

#### P02.S01 — keyboard creation and arrow-nudge for every annotation tool
Scope: every annotation can be created, moved and resized without a pointer. `ArrowUp` returns 0
across `web/app.js`, so this is the phase's live SC 2.1.1 failure and nothing about it has moved.
Refs: D11, exit criterion 1.
Acceptance:
- Each tool creates a mark from the keyboard alone, asserted at tier 3.
- Arrow keys nudge and Shift+arrow resizes the selected mark, asserted at tier 3.
- A red proof: removing the key handler turns the assertion red.

#### P02.S02 — armed state is programmatic, not colour alone
Scope: the 27 `classList.toggle('active')` sites that signal an armed tool gain `aria-pressed`, and
the panel accordion cards gain `aria-expanded`. A guard asserts the ratio cannot regress — the
measurement above is why it is a ratio and not a list. Refs: D11, exit criterion 2.
Acceptance:
- Every armed-tool toggle sets `aria-pressed` alongside its class, enumerated from the code.
- Adding a toggle without one turns the guard red, proved by adding one.

#### P02.S03 — the four buttons with no accessible name
Scope: `themeToggle`, `updateGet`, `sessionNoticeAction`, `signAction`. **Four, not 21** — and a
guard that enumerates rather than lists, so the number never needs re-measuring again. Refs: exit
criterion 3.
Acceptance:
- Every button with no visible text has an `aria-label`, enumerated from `index.html`.
- Adding one without turns the guard red.

#### P02.S04 — the keyboard-only pass, and what it finds
Scope: a tier-3 keyboard-only traversal of open → mark up → save, asserting no trap and no stranded
focus. It is last because S01–S03 are what make it passable. Refs: exit criterion 3.
Acceptance:
- Tab order reaches every interactive control and returns; no element traps focus.
- Focus is never left on a removed node after a modal closes.

**Dropped from the sketch**: the toast live region (built, `/pending 328`). `prefers-reduced-motion`,
SC 1.4.11 and the 200%-zoom reflow assertion are **not** dropped — they are unmeasured here and stay
sketched rather than being firmed on numbers nobody has taken.

### P03 — The catalog floor
**Goal.** Clear the seven PDF/UA rules that need no structure at all, and fix the `/Lang` one-door
defect. Measured: this is the largest share of the current failure for the least work.

**Exit criteria.**
- `/MarkInfo`, an XMP `/Metadata` stream, and `ViewerPreferences /DisplayDocTitle` on authored output.
- `/Lang` present from every authoring and export door, enumerated from the router rather than a
  hand-maintained list, with a guard that fails when a new door ships without one.
- The `ua1` oracle runs in tier 1 and its skip is recorded, never credited.

### P04 — Embedded fonts for authored text
**Goal.** Clear rule 7.21.4.1 for everything nib writes, and refuse honestly for everything it does
not. Refs D7.

**Exit criteria.** Authored output embeds every font it draws with; a document carrying
non-embedded fonts is refused for UA export with the reason named; `mdpdf` output passes the font
rule under veraPDF.

### P05 — The tag tree core
**Goal.** The typed model of D8 plus the wrapping emitter of D3 — parse, mutate, write back, with
`/ParentTree`, `/StructParents` and MCIDs as model invariants. This is the new capability the whole
plan rests on and the first place nib emits content-stream operators of its own.

**Exit criteria.** Round-trip of an existing tagged document is lossless; a tree built by the model
validates under veraPDF `ua1`; wrapping is proved not to disturb the wrapped content's bytes.

**Shared surface.** `PLAN-text-reflow.md`'s P05 needs the same content-stream walker, for a harder
job (rewriting operators rather than bracketing them). Whichever plan reaches it first builds it and
the other extends it; built twice, the two will disagree about the same bytes.

### P06 — Tagging what nib authors
**Goal.** Exact structure first (D4): `mdpdf` from its AST, then authored form fields with `/TU`
names and `/Tabs`, then OCR from tesseract's block/paragraph/line refs recovered across the wire.

**Exit criteria.** A markdown document, an authored form, and an OCR'd scan each pass veraPDF `ua1`;
each tree records which of D4's three sources produced it.

### P07 — The pure-Go conformance checker
**Goal.** Nib's own PDF/UA checker (D6) and the remediation report, with law 4's three verdicts and
law 5's agreement guard against veraPDF over the corpus.

**Exit criteria.** The checker agrees with veraPDF on every corpus fixture or names the rule it
cannot evaluate; no rule is reported as passing that the checker did not actually run; the report
is reachable from the UI and from the CLI.

### P08 — The autotagger
**Goal.** Heuristic structure inference for arbitrary PDFs — the research-grade half, and the one
Acrobat is actually judged on. Proposes; never asserts (law 3, D5).

**Exit criteria.** Over the corpus, proposed trees are measurably better than no tree on a stated
metric; every proposal is reviewable and editable before it is written; nothing is written silently.

**Standing caveat.** This phase carries the plan's real risk. Layout analysis is a research problem,
its quality is unbounded above, and "parity" here is a direction rather than a finish line. It is
sequenced last on purpose: everything before it ships value without it.

**Shared surface.** Grouping positioned runs into lines and paragraphs is also `PLAN-text-reflow.md`'s
P04. One rule, one door (ADR-009) — two implementations here would be two different opinions about
where a paragraph begins, in one product.

### P09 — The structure editor
**Goal.** The Tags panel and Reading Order view in the Document tab (D10) — inspect, reorder,
retype, set alt text, mark artifacts, and author table header scope.

**Exit criteria.** A tree can be corrected end to end in the UI without leaving nib; every edit is
undoable through the existing history; the panel itself meets P02's keyboard bar.

### P10 — Batch, CLI and the parity ledger
**Goal.** `nib tag` and `nib a11y-check` as headless commands composing over stdin/stdout like the
other 26, folder batch via `nib watch`, docs, and an honest written comparison of what nib does and
does not do against Acrobat feature by feature.

**Exit criteria.** The CLI covers what the UI can do; the ledger names every parity gap that
remains, with no gap silently omitted.

---

## Out of scope

- **PDF/UA-2** (D1) and **PDF/A-2a** — the archival path targets 2b, which is explicitly untagged.
- **True text reflow editing** — that is `/pending 44`, a different feature with its own declined
  prerequisites, and nothing here depends on it.
- **OCR accuracy work.** Tagging consumes tesseract's output; improving it is a separate concern.
- **Remediating documents nib did not author, silently** — forbidden by law 3, not deferred.
- **Screen-reader certification claims.** Nib may state what it conforms to and what it checked; it
  may not claim an assistive-technology endorsement it has not obtained.

## Standing caveats

- **The autotagger's ceiling is unknown** (P08). Every other phase has a measurable exit criterion;
  that one has a direction and a corpus.
- **D9 is unmeasured** at the time of writing and is the first task of P01 for that reason.
- **veraPDF is an external Java tool.** Every gate that depends on it skips cleanly when it is
  absent, and a skip is recorded as `not exercised` — a green run over an absent oracle verifies
  nothing.
- **`/pending 29` is superseded by this plan.** Three of its claims are corrected here: the `/Lang`
  slice did not ship at every door, structure is not OCR-only, and the CID/`UsedGIDs` work it
  budgeted for is not needed under D3.

## Seam inventory

`~/.claude/projects/-home-dan-repos-nib/memory/instruments/accessibility.md` — 24 rows (7 paths,
9 seams, 8 gap-downs), written against this plan's shape before any code. Its two hot-path rows
concern `/Lang` presence at the export doors; its two `diagnostic, no standing reader` rows are
pdfcpu's silent per-rune drop and `SetLang`'s best-effort failure branch.
