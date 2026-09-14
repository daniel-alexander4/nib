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

### P01 — Preservation and honesty *(done 2026-09-11, v1.129.21)*

**Acceptance ledger**, every clause split on `and`, measured at v1.129.20:

| # | clause | verdict | evidence |
|---|---|---|---|
| 1 | No operation emits a tagging claim over unmarked content | ✅ | `TestEveryDeclaredFateIsTheMEASUREDFate` fails any driven operation measuring `orphaned` — **32 driven, zero skips**. Probed red by restoring `NUp`'s v1.129.4 return. |
| 2 | `nup`'s output no longer regresses veraPDF clause 7.1 t3 against a tagged input | ✅ | The fixture n-up fails **exactly** its input's three clauses; `boi.pdf` exactly; `adgm_va.pdf` a strict **subset** of its source's ten. Two are real producer documents. |
| 3 | Every operation that touches a document has a declared tag verdict | ✅ | `TestEveryOperationDeclaresItsTagFate` — **49 operations enumerated from the code** by `go/ast`, 49 rows, checked both directions. |
| 4 | …and a guard fails when a new one has none | ✅ | Probed by omission: `StampImages` was genuinely missed in the S03 rewrite and the guard caught it first. |
| 5 | The tag-fate table and its law ship as an ADR | ✅ | ADR-031, indexed, held by `TestSupersededADRsSaySoAndEveryADRIsIndexed`. |

**Required-run gates, discharged at the close and named explicitly** — they are a separate list from
the exit criteria and nothing else walks them. Tier 0 build ✅ · tier 1 `go test ./...` ✅ · tier 2
`jsdomtest.sh` **327/327** ✅ · tier 3 `uirepro.sh` **119/119, 0 skipped** ✅ · tier 4 `pairrepro.sh`
**PASS both transports** ✅ · tier 6 `ceremonyrepro.sh` **27/0** ✅. Tiers 4 and 6 are run because the
phase widened `ClaimsTagging`, which `convene.go:244` reaches through `commitBarrier`.

**(pin — criterion 2 was UNMET when this close began, and the phase gained a sixth slice rather than
a struck clause.)** `nup` still regressed 7.1 t3 at v1.129.18. The close only got past it by asking
*what* fails that clause instead of accepting that it did: veraPDF's failing contexts read
`xObject[0]/contentStream[0]/content[2]{mcid:0}` — the original marked content, intact, inside a Form
XObject. `api.NUp` had destroyed nothing; it had only failed to re-link. P01.S06 re-anchors it, and
P01.S01's struck acceptance clause is un-struck as a consequence.

**What the phase review found, all three of the same shape as the defect the phase is built on — a
claim nothing checked.** A doc comment asserting `dropTaggingClaim` had one caller when it had two;
cost figures describing a call pattern that had stopped existing one commit earlier; and
`orphaned()` scoring a document with `/Marked true` and **no structure tree at all** as `carried`,
the census's best verdict. All fixed at v1.129.20, each probed red.

**And a count that was never true.** The census population was reported as *"28 → 33 operations"* and
*"19 of 33 rows were wrong"* across the plan, the ADR, the seam inventory and a commit message.
Counted at the close: the table held **48 rows** (39 `dropped`, 9 `untouched`), **27** of those
verdicts were false, and the enumeration finds **49** operations today.

**Seam inventory: graduated, after repair.** 22 rows — 21 `keep-live`, 1 flagged (S06d), 2 deleted
with the reverted `writeMutated` door. The repair came first: **four of section S01's five declared
readers had been deleted and the rows still named them**, because the slice-close re-check had not
run since the byte-count correction rewrote the file underneath it. `inventorycheck` passes with 23
readers resolving and six retired instruments declared.

**Residual doubt, recorded rather than resolved:** P01's strongest criterion — the veraPDF
differential — has **no standing reader**. Every run of it was by hand, and it is the only oracle in
this phase outside this repo. `/pending 469`.

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

### P02 — The application's own WCAG 2.1 AA *(done 2026-09-11, v1.129.27)*

**Acceptance ledger**, every clause split on `and`, measured at v1.129.26:

| # | clause | verdict | evidence |
|---|---|---|---|
| 1a | Every annotation tool can **create** a mark by keyboard alone | ✅ | `keyboardplacement.test.mjs` enumerates all 11 `view.*Mode` from the code; `keyboardplace.test.mjs` drives **both** code paths — click-to-place and drag-to-draw, which are different halves. |
| 1b | …**move** | ✅ | Arrow nudge, tier 3, asserted on laid-out geometry. |
| 1c | …**resize** | ✅ | Shift+arrow, asserted separately from move so neither masks the other. |
| 1d | …asserted at tier 3 | ✅ | 142/142, 0 skipped. |
| 2a | Toggle state exposed **programmatically** | ✅ | Three doors by ARIA vocabulary — `setArmed` / `setSelected` / `setExpanded`. |
| 2b | …**across every armed tool** | ✅ | Enumerated from source across **all three spellings** of the class change; 29 sites routed, 7 containers exempted by name with reasons. |
| 3a | A keyboard-only pass over **open, mark up, save** | ✅ | P02.S05 drives the whole flow with no pointer, asserted over its own source. **This clause is why the phase gained a fifth slice.** |
| 3b | …no trap | ✅ | Stall detector **with its own capability test**, because the obvious probe was inert under Nib's CSP. |
| 3c | …no stranded focus | ✅ | Asserted in the traversal, plus `dialogfocus.test.mjs` for the modal-close case. |

**Required-run gates, discharged and enumerated** — a separate list from the criteria, which nothing
else walks. Tier 0 build ✅ · tier 1 `go test ./...` ✅ · tier 2 **343/343** ✅ · tier 3 **142/142,
0 skipped** ✅. **The slice gate does NOT fire**, and that is recorded rather than skipped silently:
P02's entire diff is `web/app.js`, `test/`, `build/` and this plan — nothing in `internal/server`'s
session/ceremony/delivery/discovery, `internal/p2p` or `internal/rendezvous`.

**(pin — the phase shipped a defect and then found it, three slices later, at its own close.)**
P02.S02 gave `.modetab` `aria-pressed` after checking `index.html` for `role="tab"` and finding
none. `wireTablist` (`app.js:9008`) adds the role **at runtime**, so v1.129.23 shipped
`aria-pressed` on `role="tab"` — the invalid pairing the three doors exist to prevent. **A markup
scan cannot see a runtime decision made ten lines away in the same file.** Found by P02.S05 driving
the flow rather than reading it; fixed at v1.129.26, guard rewritten to ask the tablist-building
code, probed red. A second collision from the same blind spot put `aria-expanded` on the document
strip's tabs.

**Every slice's plan text was wrong about its own population, and each was corrected by measuring
before building:** "every annotation tool" was 11 placement tools of which 8 are drag-to-draw; "the
27 toggle sites" missed **nine more in a spelling the census did not know**; "four buttons with no
accessible name" was **two**, and two of the four would have been made worse by an `aria-label`
overriding their real text.

**Graduation pass**: 31 rows, **30 `keep-live` mechanically**, one needing judgment — S04c, whose
metric must be NON-zero because a zero there means the trap detector is dead. No rot; all seven
declared reader files resolved against the tree. **What the pass cannot see** has an instance in
this phase: a row can be live, its reader real, and the claim still false.

**Pending sweep against the closure: nothing falsified.** The two items naming "P02" are
`PLAN-signing-ceremony.md`'s and predate this plan — a coordinate collision, not relevance.
`/pending 470` is this phase's own filing and stays open.

~~**Residual doubt:** with a document open the tab order visits 1344 stops in 1500 presses and
never wraps, so the toolbar is not forwards-reachable from the document.~~ **(STRUCK 2026-09-11 at
v1.129.29 — measured, and false in every part.** The tab order is **32 elements** and cycles
perfectly: 200 presses give 32 distinct stops, the first recurs at index 32, 6.3 laps. The toolbar
is at most 32 presses away. The "1344" was a **consecutive-distinct counter recording ~47 laps of a
32-element cycle**, and the "never wraps" was a search for a mode tab that roving tabindex
deliberately keeps untabbable. `/pending 470` is overturned and closed with no fix.**)**

**So P02 closes with no residual doubt** — the one it recorded was a measurement artefact of its
own making.

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

#### P02.S01 — keyboard creation and arrow-nudge for every annotation tool *(done 2026-09-11, v1.129.22)*
Scope: every placement tool can create, move and resize a mark without a pointer. `ArrowUp` returns
0 across `web/app.js`, so this is the phase's live SC 2.1.1 failure and nothing about it has moved.
Refs: D11, exit criterion 1.

**(grill, 2026-09-11 — confirmed, amended on two counts the sketch could not see.)**

**The population is 11, not "annotations".** Eleven `els.viewerWrap.addEventListener('pointerdown')`
handlers, each gated on its own `view.*Mode` flag: `marker`, `note`, `shape`, `checkbox`,
`dropdown`, `radio`, `border`, `edit`, `crop`, `splitBox`, `redact`. The exit criterion says
*annotation* tool and the last three are document operations that happen to draw a box — but SC
2.1.1 binds every pointer-only interaction, so the slice covers all eleven and the criterion's
wording is the narrower of the two.

**Nothing knows which tool is armed.** There is no accessor; mutual exclusion is **eight copies** of
`if (view.redactMode) { view.redactMode = false; reflectRedact(); }` scattered through the arming
sites. A keyboard door has to ask "what is armed" and today that question has no single answer.

**And the sketch has no enumeration guard**, so a twelfth tool would ship pointer-only with nothing
red. That is the same hole P01.S03's census existed to close, one file over.

Tasks:
- ✅ T01 — `armedTool()`: one accessor over the eleven `view.*Mode` flags, returning the armed
  tool's name or `null`.
- ~~T02 — `PLACERS`: one row per tool — mode name, default fractional size, constructor call.~~
  **DIVERGED, and this is the slice's one real departure from its own plan.** Refused after
  reading the handlers: **eight of the eleven are drag-to-draw**, building their mark on
  `pointerup` from geometry derived *there* — an async page fetch, a viewport at scale 1, a clamp
  against the page the drag started on. A table of constructors would have reimplemented all of
  that eleven times and then drifted from it. **Replaced by synthesising a real
  `pointerdown`/`pointerup` pair on the page div**, so the keyboard mark and the mouse mark are
  identical *by construction* rather than by a test that checks they still match. Verified before
  building: no handler needs an intervening `pointermove`.
- ✅ T03 — a `keydown` door: Enter/Space with a tool armed places at the centre of the current
  page, and focuses the new mark (a one-shot request `layoutField` consumes, because placement is
  async for several tools).
- ✅ T04 — arrow nudge and Shift+arrow resize on the focused overlay, one delegated handler for
  every tool, recording undo through the existing `recordMove`.
- ✅ T05 — the guard: `keyboardplacement.test.mjs` enumerates `view.*Mode` from the source and
  requires every one to be served or exempted with a reason.
- ✅ **T06, added mid-slice** — Enter belongs to whatever has focus. See the pin below.

Acceptance:
- Each of the eleven tools creates a mark from the keyboard alone, asserted at tier 3 and
  **enumerated from the code rather than listed**.
- Arrow keys nudge and Shift+arrow resizes the focused mark, asserted at tier 3.
- **Adding a `*Mode` with no `PLACERS` row turns the guard red, proved by adding one.**
- ✅ A red proof: removing the key handler turns the creation assertion red.

**(pin — tier 3 found a defect worse than the one this slice fixes, and only tier 3 could.)** The
first version took Enter whenever a tool was armed and called `preventDefault()`. Focus, immediately
after arming a tool *with the keyboard*, is on the button that armed it — so that placed a mark and
swallowed the button's own activation. **Arming the Note tool broke every button a Tab user could
reach.** `isTypingTarget` does not cover it: it is about text entry, and a button is not a text
field. The door now returns early on `button, a[href], select, [role="button"], [role="tab"],
summary`, with its own assertion.

**And a second thing the tier taught, recorded because it re-shaped a test rather than the code:** a
placed note focuses its own `textarea`, where the arrow keys belong to the caret — correctly, and
`isTypingTarget` bails before the nudge for that reason. The nudge assertion therefore drives a
border box, which carries no text field, and the note's behaviour is pinned as its own rule so the
reasoning cannot go stale in silence.

#### P02.S02 — armed state is programmatic, not colour alone *(done 2026-09-11, v1.129.23)*
Scope: every site that signals an armed or selected control exposes that state programmatically, not
by colour alone. Refs: D11, exit criterion 2, WCAG SC 1.4.1 and SC 4.1.2.

**(grill, 2026-09-11 — confirmed, and the scope line was wrong in two directions.)**

**"gain `aria-pressed`" is wrong for part of the population.** The 27 sites are not one kind of
thing. `aria-pressed` belongs to a toggle button and `aria-selected` to something carrying a
`tab`/`option`/`row` role — and `aria-selected` on a plain `<button>` is not a weaker statement but
an **invalid** one, ignored by some readers and misreported by others. Measured: `.modetab` are
plain `<button>` with no `role="tab"`, `.tbtab` is a container `<div>`, and the sidebar's `.sbtab`
is a real tablist (`index.html:168`) already doing it correctly. So there are **three doors**, by
vocabulary, not one.

**"the panel accordion cards gain `aria-expanded`" is already shipped.** `openCard` sets it at
`app.js:11941` and `:11947`, and the headers are created with it at `:11882`. What was missing is
the `.sbhead[data-panel]` *panel* headers, which are a different path.

**And the measurement under-counted, because it knew one spelling.** 27 sites use
`classList.toggle('active', …)`; **nine more** use `classList.add('active')` / `.remove('active')`
— the same state change written differently — and that is where the panel headers live. A census
that reads one spelling has a hole the width of the others.

Tasks:
- T01 — three doors: `setArmed` (aria-pressed), `setSelected` (aria-selected), `setExpanded`
  (aria-expanded).
- T02 — route every control site through the door its vocabulary calls for.
- T03 — the guard: enumerate all three spellings from the source; every site routes through a door
  or is named, with a reason, as something that is not a control.

Acceptance:
- Every armed-tool toggle exposes its state alongside its class, **enumerated from the code across
  all three spellings**.
- Adding a toggle without one turns the guard red, proved by adding one.
- **`aria-selected` is used only where a tab role backs it**, asserted rather than assumed.
- The exemption list names only sites that still exist.

#### P02.S03 — the buttons with no accessible name *(done 2026-09-11, v1.129.24)*
Scope: every button carries a name that does not depend on hover. Refs: exit criterion 3, WCAG
SC 4.1.2.

**(grill, 2026-09-11 — the count moved again, and the same way it moved at phase-open.)** The sketch
said 21, the firmed slice said four, and the defect surface is **two**. Measured over
`web/index.html`: **279 buttons — 272 named by their own text, 3 by `aria-label`, 2 title-only
(`themeToggle`, `updateGet`) and 2 with no markup name at all (`sessionNoticeAction`,
`signAction`)**. The last two set real text at runtime and are named where it counts, so they were
never defects; a static enumeration alone would have "fixed" them with an `aria-label` that
**overrides** their real text, which is a regression dressed as a fix.

**`updateGet` was the interesting one, and worse than title-only.** It sets `textContent` to
`v1.129.23`, and text content BEATS `title` in the accessible-name computation — so its name was a
version string describing nothing, while a perfectly good sentence sat in the `title` where no
keyboard user would find it. **A button can have a name and still be unnamed in every sense that
matters**, which is why the guard does not simply ask whether a name exists.

Tasks:
- ✅ T01 — `describeButton(el, text)`: one door setting `title` and `aria-label` to the same
  sentence, because the tooltip already held the description and only the mouse user got it.
- ✅ T02 — the two title-only buttons route through it, `updateGet` at all three of its states.
- ✅ T03 — the guard: enumerate every `<button>` in `index.html`; each needs text or an
  `aria-label`, or an entry naming the `app.js` assignment that names it at runtime.

Acceptance:
- ✅ Every button has a name that does not depend on hover, **enumerated from `index.html`** with a
  floor so a scan that stops matching fails loudly.
- ✅ Adding an icon-only button with no name turns the guard red, proved by adding one.
- ✅ **`title` alone does not satisfy the rule**, and the reason is written into the test: the name
  computation does fall back to it, so a naive 4.1.2 check would pass — it is announced
  inconsistently and is invisible without a mouse.
- ✅ The runtime-named exemptions are checked **against `app.js`**, in both directions, so an
  exemption cannot outlive the code that justifies it or name a button that is gone.

#### P02.S04 — the keyboard-only pass, and what it finds *(done 2026-09-11, v1.129.25)*
Scope: a tier-3 keyboard-only traversal, asserting no trap and no stranded focus. It is last
because S01–S03 are what make it passable. Refs: exit criterion 3, WCAG SC 2.1.2 and SC 2.4.3.

Acceptance:
- ~~Tab order reaches **every interactive control** and returns~~ **(narrowed on measurement,
  2026-09-11.)** Not assertable: 279 buttons exist and most are behind a mode, a card or an open
  document, so a test claiming it would be measuring its own fixture setup rather than the app.
  **Replaced with what SC 2.1.2 actually requires and what is observable**: over the population Tab
  reaches from a real starting point, ✅ focus always moves, ✅ never stalls, ✅ never lands on
  something invisible, and ✅ the order returns to its first stop.
- ✅ **The trap detector is proved able to detect a trap**, not assumed — see the pin.
- ✅ Focus is never left on a removed node after a modal closes. Already held by
  `dialogfocus.test.mjs`, which records why this needs tier 3: jsdom's `.focus()` succeeds on
  elements inside `hidden` containers, so the assertion is green against the defect it exists for.

**(pin — the red proof for this slice was INERT, and it looked exactly like a passing one.)**
The obvious probe is to inject `onblur="this.focus()"` and watch the detector fire. It does not
fire, because **Nib's own CSP refuses inline handlers** — `script-src 'self' 'wasm-unsafe-eval'`
with no `unsafe-inline`, and the browser reports *"The action has been blocked."* The probe
produced three unrelated failures and left the detector green, which is indistinguishable from a
detector that works.

So the capability became a **standing test** rather than a one-off: it installs a trap the way the
app could actually acquire one — a listener added through the DOM API, which is what a real
focus-management bug is — and requires the stall to be caught. That test goes red the day the
detector goes inert, which no amount of hand-probing can promise.

**And the detector's first version reported a trap that was not there.** It keyed element identity
on `id || className`, and every accordion header is `class="sbhead groupcard"` with no id — so
three *different* buttons in a row read as one element. An identity function that cannot tell two
elements apart turns "focus moved" into "focus is stuck". Re-keyed on a DOM path.

#### P02.S05 — the flow itself, by keyboard alone *(done 2026-09-11, v1.129.26)*

**(added 2026-09-11 at the phase close, because the acceptance ledger stopped on exit criterion 3
and the phase would otherwise have closed on a narrowed version of its own words.)** The criterion
says *"a keyboard-only pass over the primary flows — **open, mark up, save**"*. S04 traverses the
app with a document **already open** and asserts no trap and no stranded focus; it never drives the
flow. Those are different claims, and only the weaker one was tested.

**It is achievable, which is why this is a slice and not a struck clause.** Checked before scoping
it: `#pathInput` takes a path and `Enter` calls `openTyped` → `openSmart` (`app.js:8837`), so Open
has a real keyboard route that does not go through the browser's native file picker. `#saveBtn` is
an ordinary button. Mark-up by keyboard is P02.S01's, already shipped.

Scope: one tier-3 test that opens a document, places and moves a mark, and saves — **using only
keys**, never `page.click` or a harness helper that reaches past the UI. Refs: D11, exit criterion 3.

Tasks:
- T01 — open by keyboard: reach the Open dialog, type the path, `Enter`.
- T02 — mark up by keyboard: arm a tool with `Enter`, place with `Enter`, nudge with an arrow.
- T03 — save by keyboard: reach `#saveBtn` and `Enter`; assert the document reports saved.
- T04 — the stimulus check: the test must fail if any step silently used a pointer.

Acceptance:
- The whole flow completes with **zero `page.click` and zero `page.mouse` calls** in the test,
  asserted over the test's own source so the claim cannot rot into a mouse-driven test that says
  it is keyboard-only.
- Each step's effect is observed, not assumed: a document is open, a mark exists and has moved,
  and the save is reported.
- ✅ A red proof: disabling the keyboard route for any one step turns it red — and four of the
  five runs it took to get here were exactly that, unplanned.

**(pin — this slice found a defect P02.S02 SHIPPED, and the guard that should have caught it was
asking the wrong artefact.)** `wireTablist` (`app.js:9008`) gives `.modetabs` `role="tablist"`,
each tab `role="tab"`, `aria-selected` and a roving tabindex — **at runtime**. S02 checked
`index.html`, found the mode tabs are plain `<button>`, and routed them through `setArmed`. So
v1.129.23 put **`aria-pressed` on elements carrying `role="tab"`**: the invalid pairing the three
doors exist to prevent. A markup scan cannot see a runtime decision made ten lines away in the
same file, and the guard is now asked of the tablist-building code instead.

**A second collision from the same blind spot**: `.tab` matches both the sidebar's panel headers
and the document strip's tabs (`app.js:2509`), which `wireTablist` also runs as a tablist — so
`setExpanded` was setting `aria-expanded` on a real tablist. Scoped to `.tab[data-panel]`. Each
door now clears the other two's attribute, so a mis-routed element cannot keep a stale vocabulary.

**And the test's own assumption was wrong in the instructive direction.** It expected Tab to reach
each mode tab; a tablist is **one** Tab stop with arrows moving inside it, which is correct ARIA and
what the app does. The failure read as *"the app's primary navigation is keyboard-unreachable"* —
the Level A defect this phase exists to find — and took two runs to tell apart, only because the
failure message lists the stops focus actually visited.

~~**Measured and filed rather than fixed here:** 1500 forward `Tab` presses visit 1344 distinct
stops and never wrap.~~ **(STRUCK 2026-09-11 — the follow-up measurement refuted it.** The tab
order is **32 elements** and cycles at index 32; the text layer holds **zero** focusable elements
at 1, 2 or 6 pages and the count does not scale with page count. The figure came from a counter
that recorded a stop whenever it differed from the previous one, which over ~47 laps of a small
cycle reads as 1344 different places. `/pending 470` closed, overturned.**)**

**Dropped from the sketch**: the toast live region (built, `/pending 328`). `prefers-reduced-motion`,
SC 1.4.11 and the 200%-zoom reflow assertion are **not** dropped — they are unmeasured here and stay
sketched rather than being firmed on numbers nobody has taken.

### P03 — The catalog floor *(done 2026-09-11, v1.129.37)*
**Goal.** Clear the PDF/UA rules that need no structure at all, and fix the `/Lang` one-door
defect.

**(phase-open, 2026-09-11 — measured against a nib-authored document, and the sketch's count and
one of its three criteria are both wrong.)** `CreateFromJSON`'s output through veraPDF ua1 fails
**seven** clauses, and only **four** of them are catalog-only:

| clause | what it wants | needs | whose |
|---|---|---|---|
| 7.1 t8 | an XMP `/Metadata` stream in the catalog | catalog only | **P03** |
| 7.1 t10 | `ViewerPreferences /DisplayDocTitle true` | catalog only | **P03** |
| 7.2 t34 | *"natural language for text in page content shall be determined"* — `/Lang` | catalog only | **P03** |
| 6.2 t1 | `/MarkInfo` with `/Marked true` | catalog only | ⚠ **moved to P05 — see below** |
| 7.1 t3 | content marked as artifact or tagged | a structure tree | P05 |
| 7.1 t11 | logical structure rooted in `/StructTreeRoot` | a structure tree | P05 |
| 7.21.4.1 t1 | font programs embedded | embedded fonts | P04 |

**So "the seven rules that need no structure at all" is three, plus a fourth that must not be
done here.** The other three are P04's and P05's, and the split falls exactly on the phase
boundaries the plan already has — which is the sketch being right about the shape and wrong
about the number.

**`/MarkInfo` is struck from this phase's floor, and the reason is a law this plan adopted after
the sketch was written.** Setting `/Marked true` with no `/StructTreeRoot` is precisely
`tagState.orphaned()` — a document asserting tagging it has not got — which **ADR-031's law 1
forbids** and which P01.S06 built a door to prevent. Clearing 6.2 t1 on its own would ship the
state P01 exists to stop, and would be caught by P01's own guard. `/MarkInfo` therefore arrives
**with the tree**, in P05, and the two clauses move together.

**Exit criteria** *(amended at phase-open, 2026-09-11)*:
- ~~`/MarkInfo`,~~ an XMP `/Metadata` stream **carrying `dc:title`**, and `ViewerPreferences
  /DisplayDocTitle` on authored output — clearing ua1 7.1 t8 and 7.1 t10, measured as a before/after
  on the same document rather than asserted.
- `/Lang` present from every **authoring** door — the operations that CREATE a document, which
  P01.S03's census already enumerates as `untouched` because their input is not a PDF — enumerated
  from the code rather than a hand-maintained list, with a guard that fails when a new one ships
  without it. Clears 7.2 t34.
- The `ua1` oracle runs in tier 1 and its skip is recorded, never credited. **This criterion is
  `/pending 469`**, filed at P01's close before anyone noticed the plan already schedules it here.

**Acceptance ledger — phase close, 2026-09-11 at v1.129.37.** Every clause split on `and`, asked
separately.

| # | clause | verdict |
|---|---|---|
| 1a | an XMP `/Metadata` stream | ✅ `TestSetTitleWritesTheWholeCatalogFloor`, read back through a validating re-read |
| 1b | carrying `dc:title` | ✅ same, plus `TestSetTitleEscapesAHostileTitle` — the packet must PARSE, not merely contain a string |
| 1c | `ViewerPreferences /DisplayDocTitle` | ✅ same; recorded as a red proof, because a title nothing displays passes every presence check |
| 1d | **on authored output** | ✅ three receiving sites route through the door; three fragments named with measured reasons (`titledoor_test.go`) |
| 1e | clearing ua1 7.1 t8 and 7.1 t10 | ✅ measured before/after on the same bytes. **7.1 t9 passes in the same step** — with no `/Metadata` there was no stream for it to inspect |
| 1f | measured rather than asserted | ✅ the table in S01, and now `TestNoOperationAddsAUA1ClauseItsInputDidNotFail` re-measures it every run |
| 2a | `/Lang` present from every authoring door | ❌ **NOT MET, and refuted as a goal.** Three of six doors have no language anyone determined, and inventing one is a false statement about the document. S02 carries the measurements |
| 2b | enumerated from the code, not a hand-maintained list | ✅ `authoringscan_test.go`, shared with 1d's guard so the two cannot drift |
| 2c | a guard that fails when a new door ships without it | ✅ **amended**: it demands a recorded ANSWER (`declares` / `carries` / `none` / `fragment`) rather than a write, and cross-checks the `declares` claim against the code. Proved by adding a door |
| 2d | clears ua1 7.2 t34 | ❌ for markdown; **never fires** on raster output (no text in page content); cleared on the office path by a `/Lang` that was already there |
| 3a | the ua1 oracle runs in tier 1 | ✅ 2.2 s for the whole census, one veraPDF invocation |
| 3b | its skip is recorded, never credited | ✅ the skip names the criterion it leaves unchecked; probed by making `verapdfPath()` return `""` |

**Criterion 2 was written on a premise measurement refuted, and the phase says so rather than
scoring itself green.** *"`/Lang` present from every authoring door"* assumes a language is
available to each door. It is not: the office path carries the **converting machine's locale**, the
markdown path renders text nib was never told the language of, and raster output has no text at all.
What the phase delivered instead is the enumeration and the recorded answer — which is what makes
the gap actionable (`/pending 471`) rather than invisible.

**Three defects were found and fixed that were on no slice's list**, each by a different mechanism:
two of S01's titles were written onto fragments whose catalogs `Append` discards (found by S02's
step zero); four page operations dropped the catalog `/Lang` while `carryLang` sat two functions
away with two callers (found by S03's oracle on its first run, `/pending 472`); and all three
receiving sites let a failed metadata write fail the whole operation, one of them as a **400**
(found by this phase-close review, reading the diff).

**Two gaps are filed where they can be acted on**: `/pending 471` (nobody tells nib what language a
document is in) and `/pending 473` (every stamping operation emits an optional-content dictionary
ua1 7.10 forbids by name). One is carried into P06: nib's English prose stapled into a document
whose `/Lang` says otherwise, which no catalog key can fix.

**Firmed slices:**

#### P03.S01 — the catalog floor that does not lie *(done 2026-09-11, v1.129.30)*

**(grill, 2026-09-11 — confirmed, and the population is five call sites, not "authored output".)**
pdfcpu **reads** XMP and has no writer (`model/metadata.go` unmarshals only), so the packet is
hand-built. `ViewerPreferences` is a model struct with `DisplayDocTitle *bool` and
`BindViewerPreferences()`, so that half is supported.

**`dc:title` is the whole design question, and it decides the shape.** PDF/UA 7.1 t9 wants a title
that *clearly identifies the document*; a generic one baked into the primitives — "Document" — is
the failure wearing a pass, and the primitives do not know what they are making. **Each caller
does.** So the door takes a title and the callers supply theirs, rather than the primitive
inventing one.

**Six call sites author a document; five of them produce one somebody receives.**
`internal/pdfops/pdfops.go:374` calls `ImagesToPDF` to build a one-page raster **fragment** inside
`RedactPages` — not a document, and it must not get a title. That is the exemption the guard needs,
and it is the kind of site a population defined as "every call of an authoring primitive" would have
swept in.

Tasks:
- T01 — `SetTitle(pdf, title)`: one door writing an XMP `/Metadata` packet with `dc:title`, the
  Info dict's `/Title` to match, and `ViewerPreferences /DisplayDocTitle true`.
- T02 — the five receiving call sites pass their own title.
- T03 — the guard: enumerate authoring call sites from the code; each routes through the door or is
  named, with a reason, as a fragment.
Scope: authored output carries an XMP `/Metadata` stream with `dc:title` and
`ViewerPreferences /DisplayDocTitle true`. **Not `/MarkInfo`** — see the phase pin. Refs: exit
criterion 1, ADR-031 law 1.
Acceptance *(amended mid-slice on measurement, 2026-09-11)*:
- A nib-authored document no longer fails ua1 **7.1 t8** or **7.1 t10**, measured before and after
  on the same bytes.
- ~~It gains no clause it did not already fail — the same differential shape P01.S06 uses.~~
  **(STRUCK — the shape is wrong here, and measuring it is what showed why.)** Supplying a metadata
  stream **creates the object that other clauses inspect**: with no `/Metadata` at all, ua1 5 t1
  ("the stream shall carry the PDF/UA Identification schema") and 7.2 t33 ("natural language for
  document metadata shall be determined") have **no subject and are not evaluated**. Adding an
  honest one makes both applicable. A differential that counts clauses therefore penalises
  supplying an artefact that was missing, which is the opposite of what it is for. **Replaced
  with the two dispositions below**, which name every clause the change makes applicable.
- ✅ **7.2 t33 is cleared by a `/Lang`, and this slice does not write one** — measured: title
  alone leaves it failing, title + `/Lang` clears it **and** 7.2 t34 together. ~~The two slices are
  coupled and the phase's benefit lands at S02.~~ **(AMENDED at S02's grill, 2026-09-11.)** S02
  turned out to write no `/Lang` at all, because no authoring door can determine one. So t33 is
  cleared only where a `/Lang` is **already present** — the office conversion path, where
  LibreOffice supplies it — and that document, titled, fails exactly **one** ua1 clause: `5 t1`,
  the one nib refuses. For `CreateFromJSON`, markdown and raster output, t33 stays failing and the
  phase does not claim otherwise. S01 alone is a net zero by clause count on those, and saying so
  is the honest read.
- ✅ **5 t1 is REFUSED, not deferred.** Clearing it means writing `pdfuaid:part` into the packet —
  a **conformance assertion**, which is the third thing ADR-031's law 1 forbids by name over
  content that is neither tagged nor marked as an artifact. The document still fails 7.1 t3 and
  7.1 t11; asserting PDF/UA over it would be the same lie `/MarkInfo` would have been at
  phase-open, in a different field. It arrives in P05 when it is true.
- `inspectTags` still reports the document un-orphaned: the floor must not make it claim tagging.
- A red proof: removing either key turns its clause red again.

**Measured on the shipped code, same bytes through veraPDF ua1** (`CreateFromJSON` output):

| | failing clauses |
|---|---|
| before | `6.2 t1` `7.1 t3` **`7.1 t8`** `7.1 t11` **`7.1 t10`** `7.21.4.1 t1` `7.2 t34` |
| after `SetTitle` | `5 t1` `6.2 t1` `7.1 t3` `7.1 t11` `7.21.4.1 t1` `7.2 t33` `7.2 t34` |
| after `+ /Lang` *(S02's, shown for the coupling)* | `5 t1` `6.2 t1` `7.1 t3` `7.1 t11` `7.21.4.1 t1` |

**7.1 t8 and 7.1 t10 are cleared, and 7.1 t9 never appears in either column** — with no `/Metadata`
there is no stream for it to inspect, so it is the third clause the change makes applicable and
passes in the same step.

**The population came out at six sites, not five, and the sixth is a caller the slice did not
plan for.** `handleAssemble` takes its title from the OPEN document rather than from a file name —
it builds from posted raster images and has no filename of its own. The name is read off the
advisory resolve at the top of the handler, as a string and not a held document, so the warning
there about not reusing the resolved document as a commit target stands whole.

**`TitleFromFilename` returns `""` rather than a fallback, and the callers guard on it.** A
document whose name derives to nothing gets no title: `"Document"` is precisely the generic label
7.1 t9 exists to refuse, so failing the clause truthfully beats passing it falsely.

**CORRECTION, 2026-09-11 at v1.129.32 — the receiving population is THREE, not five, and two of
S01's titles were inert.** Found at P03.S02's step zero, which had to ask where each authoring
door's `/Lang` would end up. `RenderReadme` and `renderPage` are **fragments**: their only
production callers hand them to `pdfops.Append` as the *second* argument, and `Append`'s own doc
comment already said what a measurement then confirmed — `MergeRaw` takes the **first** document's
catalog whole. So a title written on either was discarded on every path that exists.

**Nothing shipped broken, and the check for that is the interesting half.** If `Append` took the
second catalog instead, a user's own contract would have been silently renamed *"About this
co-signed document"* by a page nib stapled to the back. Measured both ways — titled first document
and untitled — and both keep the first. `TestAppendKeepsTheFirstDocumentsCatalog` now holds that
claim, because it is a claim about **pdfcpu** rather than about nib and is exactly the kind that
goes stale silently.

So the guard carries **three** named fragment exemptions and **three** receiving sites
(`office.go`, `commands.go`, `export.go`). The co-signed document's title is the user's document's
own, which nib does not invent — S01's authoring-only rule, reaching further than it looked.

**Measured, the whole arc of this phase on one authored document:** 7 failing clauses → 5 after
S01+S02. The five left are P04's (7.21.4.1 t1, fonts), P05's (6.2 t1, 7.1 t3, 7.1 t11) and the
one nib refuses (5 t1).

#### P03.S02 — where `/Lang` can honestly come from, and where it cannot *(done 2026-09-11, v1.129.33)*

**(grill, 2026-09-11 — OVERTURNED on its own premise, and the scope is now the enumeration rather
than the write.)** The slice was written as *"every authoring operation sets `/Lang`"*. Four
measurements refute it, and each one removes a door from the population:

| door | `/Lang` today | who determines the language |
|---|---|---|
| `office.go` · `commands.go` → `ConvertDocToPDF` (office) | **`en-US`, from LibreOffice** | **the converting machine, not the document** — see below |
| the same doors → `ConvertDocToPDF` (`.md`, pure Go) | none | the user. nib has no basis |
| `export.go` → `ImagesToPDF` | none | nobody: raster pages, **no text in page content at all**, so 7.2 t34 never fires on this output |
| `readme.go` · `sigpages.go` → `CreateFromJSON` | none | nib knows exactly — its own English prose — and it is **inert**, because `Append` discards a fragment's catalog (the v1.129.32 correction) |
| `RedactPages` → `ImagesToPDF` | n/a | a fragment |

**The office door's `/Lang` is the CONVERTING MACHINE's locale, and finding that out took four
measurements past the obvious one.** The first answer was *"LibreOffice derives it from the source
document"*, which is what a single reading of `en-US` on an English document supports. Then: three
DOCX files declaring `w:lang` `de-DE`, `th-TH` and `fr-FR` → **all `en-US`**; an ODT declaring
`fo:language="de"`, in LibreOffice's own native format, so a parse failure cannot be the
explanation → **`en-US`**; and the same ODT re-converted under `LANG=de_DE.UTF-8` → **`de-DE`**.
The value tracks the machine, not the file.

**nib passes it through rather than stripping it, and that is a decision.** Stripping would take a
correct declaration off every document whose author and machine share a language — the common case
— to avoid a wrong one where they do not, and would move the office path from **one** failing ua1
clause to three. Correcting it needs someone who actually knows, which is the user; that is the
`/pending` item this slice files rather than a default this slice invents.

**So `SetLang`'s single caller is not a one-door defect; it is the one place nib knows the answer.**
`/pending 29` reason 2 reads the count as an oversight. Measured, the OCR path is the only site
where a language has been *determined* — by the user, choosing what to recognise — and every other
door either already carries a better determination or has none available to it.

**Why the gap is left open rather than filled with a default.** Writing `"en"` on a document nib
did not write the text of is a false statement about that document, which is ADR-031 law 1's
principle in a different field; an absent `/Lang` is silence. (Secondary, and NOT measured here:
assistive technology with no `/Lang` falls back to the reading user's own setting, so silence is
also the better outcome — stated as reasoning about AT behaviour, not as a result.)

**The one real language defect nib creates is not a catalog key.** `AppendReadme` staples nib's
English prose into a document whose `/Lang` may say `de`, and from that moment nib's own text is
declared German. The catalog cannot fix it — the fix is a language on the *content*, and P05 must
wrap that exact text in structure elements anyway. Filed there rather than done twice.

Refs: exit criterion 2, `/pending 29` reason 2.

Tasks:
- T01 — the guard: enumerate authoring doors from the code and classify each `declares` /
  `carries one already` / `no determination exists`, every row with its reason. A new door with no
  classification turns it red.
- T02 — the behavioural half the source scan cannot see, two standing readers, skip-guarded on
  LibreOffice and **reporting** the skip: the converter's `/Lang` does not come from the document
  (asserted as an equality between two sources declaring different languages, so it needs no
  particular locale installed), and nib's pipeline does not replace it.
- T03 — the two gaps recorded where they can be acted on: the appended-prose language into P05,
  and a user-declared language for markdown and raster output as a `/pending` item gated on P05.

Acceptance *(rewritten at the grill, 2026-09-11)*:
- The authoring doors are enumerated **from the code** with a floor, and every one carries a
  classification with a reason.
- A new authoring door with none turns the guard red, proved by adding one.
- An office conversion's `/Lang` survives nib's path unchanged — measured, not asserted.
- The claim that value is *not* the document's own language is a standing reader, not a note.
- ~~A nib-authored document no longer fails ua1 7.2 t34.~~ **(STRUCK.)** It is not cleared for
  markdown or raster output and this slice says why rather than claiming it: no door can determine
  a language it was never given. It never fires on raster output at all.

#### P03.S03 — the ua1 oracle becomes a standing reader *(done 2026-09-11, v1.129.35)*
Scope: the veraPDF ua1 differential runs in tier 1, `t.Skip`-guarded, and its skip is **reported**
rather than passing silently. Closes `/pending 469`. Refs: exit criterion 3.

**(grill, 2026-09-11 — confirmed, with one amendment the slice could not have known before it ran,
and two findings.)**

**Why it was never wired, and why it now costs 2 seconds.** veraPDF is a JVM: ~1.5 s per
invocation, which over a 49-operation census is three minutes. It takes **many files in one
invocation** — measured, 12 files in 2.6 s against 1.5 s for one, roughly 90 ms per extra file once
the JVM is up. The whole census is one batch, and the test runs in **2.2 s**.

**The population is the census.** It drives `tagFates`, which ADR-031 law 2 already enumerates from
the code with its own stale-row check and floor. A hand-kept list here would be a second population
that drifts from the first.

**AMENDED: the assertion is a table of deltas, not "adds nothing".** P01 struck that acceptance for
itself and wrote down why — *dropping is what law 1 demands and necessarily ADDS failures, since
PDF/UA requires a tree, so a ua1 failure count scores honesty as a regression.* An operation
declared `dropped` gives up the tree on purpose and `6.2 t1` / `7.1 t11` / `7.1 t3` are what a
missing tree looks like to a clause counter; a guard failing on those would be demanding the
dishonest option. So the rule is **"adds nothing that is not written down"**, checked both ways:
a clause that appears is a regression, and a clause that stops appearing means something was fixed
and the row is a claim about code that no longer exists.

**Eighteen deltas were recorded on the first run, and two of them are defects.** Filed rather than
folded into this diff, so the table is the record that the instrument found them:

- **`/pending 472`** — ~~five~~ **four** page operations drop the catalog **`/Lang`** as well as the
  tree. **CLOSED the same day, v1.129.36**: `splice` (`InsertPDF`, and `SplitPage`/`SplitRegions`
  through `replacePage`) and both of `Crop`'s exits now route through `carryLang`, which sat at
  `pdfops.go:193` with **two** callers — ADR-009's shape exactly. `CarryAttachments` was in the
  filed item until its drive call was read: the census hands it a genuinely different destination
  document, so having none of the source's `/Lang` is correct. **A differential reports what it
  measured, not what it measured it on.** The fix proved itself — `knownUA1Deltas` is checked both
  ways, so the four rows had to SHRINK before the suite went green.
- **`/pending 473`** — all four stamping operations emit an optional-content configuration
  dictionary with `/AS` present and `/Name` missing, which ua1 7.10 t2 and 7.10 t1 forbid by name.
  Two catalog keys, in every stamped document nib produces.

The rest are structural and named: merges bring untagged pages (`7.1 t3`), form fields and
annotations arrive without `/TU` or `/Contents` (**P06's stated goal, now measured**), and
`SetTitle` makes `5 t1` *applicable* by supplying the stream it inspects — refused until P05, when
asserting it would be true.

Acceptance:
- ✅ A tier-1 test asserts an operation's output fails no ua1 clause its input did not — **amended
  above** to "none that is not recorded with a reason", which is the only form that does not score
  honesty as a regression.
- ✅ With veraPDF absent it SKIPS and says so; the skip is visible in the run, never credited as a
  pass — the failure mode `/pending 411` records, where three seed tests reported SKIP on a
  silently-always-true condition. Probed by making `verapdfPath()` return `""`.
- ✅ A red proof: an operation that adds a clause turns it red. Five arms probed — a new clause with
  no row, a recorded delta that shrinks, a stale row, an `unvalidatable` row whose output
  validates, and the skip.

### P04 — Embedded fonts for authored text *(done 2026-09-11, v1.129.44)*
**Goal.** Clear rule 7.21.4.1 for everything nib writes, and refuse honestly for everything it does
not. Refs D7.

**Exit criteria** *(amended at the phase close, 2026-09-11)*: Authored output embeds every font it
draws with; ~~a document carrying non-embedded fonts is refused for UA export with the reason
named~~ **(MOVED TO P07 — there is no UA export door, by named search, and a refusal for an export
nothing performs is a remedy nobody can run; see S04)**; `mdpdf` output passes the font rule under
veraPDF.

**(phase-open, 2026-09-11 at v1.129.38 — five measurements, and two of them change the phase.)**

| measured | result |
|---|---|
| what `mdpdf` draws with | **five** Base-14 core faces: `Helvetica`, `-Bold`, `-Oblique`, `-BoldOblique`, `Courier` (`mdpdf/mdpdf.go:36-41`) |
| what is available to embed | **`Roboto-Regular` and nothing else.** `font.IsUserFont` is false for `Roboto-Bold`, `-Italic`, `-BoldItalic` and `RobotoMono-Regular`; the thirteen faces nib vendors are all non-Latin script faces |
| what embedding costs a document | 1,453 → 29,000 bytes for a one-line page — a real subset, with `/ToUnicode`, as `JAZKYI+Roboto-Regular` |
| what embedding clears | **7.21.4.1 t1 goes** — and **7.21.4.2 t2 arrives** |
| what vendoring costs the binary | nib already ships **7.4 MB of fonts** in a **100 MB binary**. Three Roboto styles plus a mono face is ~680 KB: **0.68% of the binary, 9% of the font payload it already carries** |

**So the binary-size question dissolves before it is asked**, and the phase's real obstacles are the
two nobody had named:

- **There is no embeddable bold, italic, bold-italic or monospace.** pdfcpu bundles exactly one
  Latin face. Clearing 7.21.4.1 for `mdpdf` therefore means *vendoring*, not *switching*.
- **Embedding introduces a clause of its own.** ua1 7.21.4.2 t2 — *"if the FontDescriptor of an
  embedded CID font contains a CIDSet stream, then it shall identify all CIDs present in the font
  program, regardless of whether a CID is referenced or used"*. Measured on the subset above: the
  `/CIDSet` is 162 bytes with **16 bits set**, which is the used-glyph count, not the program's. The
  clause is **conditional on the stream being there at all**, and PDF/UA does not require it.

**And one measurement removes work D7 assumed.** D7 says *"core-font metrics stay for layout"*.
`mdpdf` already has both paths and picks between them — `style.width` calls `font.TextWidth` by RUNE
for an embedded face and `CoreWidth` (byte-encoded) for a core one, with the reason written out at
`mdpdf/layout.go:33-44`. Layout follows the face automatically. **The place that does not is
`internal/p2p`**, whose readme and signature-page wrappers call the exported `CoreWidth`, whose own
doc comment says *"Core fonts only … a caller with a fallback face wants that path, not this one"* —
and `ErrReadmeOverflow` refuses a body that runs past the page, so a metric change there is a
page-fitting change with a live refusal already watching it.

**Acceptance ledger — phase close, 2026-09-11 at v1.129.44.**

| # | clause | verdict |
|---|---|---|
| 1a | authored output embeds every font it draws with — **Markdown** | ✅ no font clause at all. Read out of the produced document by `nonEmbeddedFonts`, with a stimulus floor asserting the opposite on the core path |
| 1b | the same — **nib's own co-signing pages** | ✅ the readme and the signature pages report no non-embedded font and carry no `/CIDSet` |
| 1c | the same — **an office conversion** | ✅ unchanged and already true: LibreOffice embeds, and a converted document fails neither font clause (measured at P03) |
| 2 | a document carrying non-embedded fonts is refused for UA export | ➖ **MOVED TO P07.** There is no UA export door — named search, four hits, all in test files — and a refusal for an export nothing performs is a remedy nobody can run. S04 carries the reasoning |
| 3 | `mdpdf` output passes the font rule under veraPDF | ✅ **7.21.4.1 t1 cleared, and 7.21.4.2 t2 with it** — the clause embedding introduced. nib's Markdown output now fails `5 t1` (refused by decision), the three tree clauses (P05) and the two language clauses (`/pending 471`) |

**What the phase found that no slice predicted**, each by a different mechanism:

- **There is no embeddable bold.** pdfcpu bundles exactly one Latin face, so the phase was a
  vendoring job rather than a switching job (S01, at phase-open).
- **Embedding creates a clause.** `7.21.4.1` out, `7.21.4.2` in — and pdfcpu's own comment is the
  diagnosis (S02, from the first measurement).
- **The pdfcpu entry points are THREE.** `CreateFromJSON` was outside S02's population, and the
  readme failed the new clause the moment it embedded a face. Found by measuring the page (S05).
- **`readmeFont` was serving two purposes** — the PDF's face and a proxy for a *browser canvas* that
  draws the acceptance block as an image. Following it would have changed which lines get truncated
  in a signed document (S05).
- **Four tests were about to assert nothing.** The old extractor decoded single-byte WinAnsi; the
  pages are now Type0/CID. The load-bearing assertions are NEGATIVE, so garbled text passes them
  silently — caught only by a setup guard those helpers already carried (S05).
- **An unwritable font directory failed every conversion, and always had** (S03).
- **The tail more than doubled a shared primitive.** Found at this phase close by benchmarking
  rather than reading: `CreateFromJSON` 3.8 ms → 8.7 ms, on a door called once per signature page,
  for a check that is a no-op on any Base-14 spec. Now asked of the spec, and back to 3.8 ms.

**Firmed slices:**

#### P04.S01 — there is no embeddable bold, and that is the phase's first problem *(done 2026-09-11, v1.129.40)*
Scope: vendor `Roboto-Bold`, `Roboto-Italic`, `Roboto-BoldItalic` and a monospace face, install them
through the door `InstallOCRFonts` already uses for thirteen, and point `mdpdf`'s five typography
constants at them. Refs D7, exit criterion 1.

Tasks:
- T01 — vendor the four faces plus `Roboto-Regular`, with their notices entries.
- T02 — `mdpdf.Faces` / `ConvertWithFaces`: the base faces become a parameter, the way the fallback
  pool already is, and a partial set degrades to Base-14 rather than refusing.
- T03 — `pdfops.authoringFaces()` supplies them, and the Markdown door uses it.
- T04 — the guard, read out of the OUTPUT rather than from a list of names.

**Five faces are vendored, not four.** `Roboto-Regular` is vendored too, even though pdfcpu installs
its own copy, so all five come from ONE place and succeed or fail together — without that, an
install failure leaves a document set in a vendored bold and pdfcpu's regular.

**The monospace face is `LiberationMono`, and its name is not its file name.** pdfcpu registers a
face under the PostScript name inside the TTF and **ignores the name it is given**, so
`LiberationMono-Regular` installs a face nothing can reference. `mdpdf.installFallbacks` refuses
that mismatch rather than installing under a dead name, and that refusal caught it on the first run
— the same split `DroidSansFallback` already carries.

**Measured, and the size is the part worth stating.** A one-page Markdown conversion goes from
**1,175 bytes to 239,080** — 233 KB of it font programs, five subsetted faces. It is not the choice
of family: the same document through a full Liberation Sans set comes out at 233,325 bytes of font
program against Roboto's 233,717, so this is **pdfcpu's subsetter**, not the faces, and no other
vendored family would do better. Roboto stays because it matches pdfcpu's own bundled Regular.

**And one thing this slice does NOT do.** `internal/p2p`'s readme and signature pages are authored
output too, and they still name `Helvetica`/`Helvetica-Bold` in their `CreateFromJSON` specs. They
are **P04.S05**, because they wrap text with `mdpdf.CoreWidth` and `ErrReadmeOverflow` refuses a
body that runs past the page — a metric change there is a page-fitting change with a live refusal
already watching it.

Acceptance:
- ~~`font.IsUserFont` is true for every face `mdpdf` names, **enumerated from `mdpdf`'s own
  constants**.~~ **(AMENDED — the constants are the wrong population.)** The faces are now a
  parameter, so a name list would pass while a sixth face went unsupplied. Read out of the OUTPUT
  instead: `nonEmbeddedFonts` walks the produced document's own font dictionaries, so a face nobody
  remembered shows up as what it is. ✅ `TestAuthoredMarkdownEmbedsEveryFontItDrawsWith`, with
  `TestTheCoreFacesAreStillTheFallbackAndStillFail` as the stimulus floor asserting the OPPOSITE on
  the same input — without it the guard passes on a build where `nonEmbeddedFonts` reports nothing.
- ✅ `mdpdf` output no longer fails ua1 7.21.4.1 t1, measured before and after on the same source.
  7.21.4.2 t2 arrives in its place and is **S02's**.
- ✅ The `style.width` split still routes, **and the flag reaches the styles** — which the first
  version of the test did not show, and mutation found: `faceSet.sty` returning `embedded: false`
  left it green, because the test constructed its styles by hand.

#### P04.S05 — the co-signing pages are authored output too *(done 2026-09-11, v1.129.43)*
Scope: `internal/p2p`'s readme and signature pages name `Helvetica` and `Helvetica-Bold` in their
`CreateFromJSON` specs (`readme.go:106-107`, and `$body` in both files), so every co-signed document
carries non-embedded fonts on the pages **nib itself wrote**. Refs exit criterion 1.

**The risk is page fit, not fonts.** Both wrap with `mdpdf.CoreWidth` — whose own doc comment says
*"Core fonts only … a caller with a fallback face wants that path, not this one"* — and
`ErrReadmeOverflow` refuses a body that runs past the page. Changing the face changes every width,
so the readme can start overflowing where it did not.
**Three things this slice found that the firming did not predict.**

1. **`readmeFont` was serving two different purposes.** `attestation.go` measured the *acceptance
   block's* text against it — and that block is **rasterised by the browser** onto a canvas at
   `px sans-serif` (`web/app.js`, `renderAttestation`) and stretched in as an IMAGE. Nothing about
   it is a PDF text run. Following the readme's face there would have re-measured a browser canvas
   against a font the browser is not using, silently changing which acceptance lines get truncated
   in a document people sign. It is now `blockProxyFont`, with the reason at the constant.
2. **`CreateFromJSON` is a THIRD door P04.S02's rule had to reach.** The readme came out failing
   7.21.4.2 t2 the moment it embedded a face. nib enters pdfcpu three different ways — mdpdf's own
   `api.Create`, the OCR watermark path, and `CreateFromJSON` — so there are three tails, all
   routing through one door. Found by measuring the page, not by reading the call graph.
3. **Four tests' extraction broke, and that is the most useful finding.** `internal/p2p` and
   `internal/ceremony` both decoded the raw content stream as single-byte WinAnsi, which was correct
   for core fonts and returns `\x007\x00M\x00K…` for an embedded Type0/CID face. **The document is
   fine** — `pdftotext` reads the rendered readme in full — so the defect was in the instrument.
   Every assertion built on that text is a substring check and the load-bearing ones are NEGATIVE
   (*"the page no longer says two people"*), so a garbled extraction fails the positive ones loudly
   and **passes the negative ones silently**. The setup guard those helpers already carried is the
   only reason this surfaced as a failure rather than as four tests quietly asserting nothing. Both
   now use `pdftotext`, the oracle this repo already uses for embedded-font extraction
   (`internal/pdfops/ocr_test.go`), skipping loudly when it is absent.

**The readme fits: 31 lines either way, 95 pt of slack.** Roboto and Helvetica happen to wrap this
prose to the same line count, which is luck rather than design — the slack is under seven lines and
`ErrReadmeOverflow` is what stands between a prose edit and an illegible page.

Acceptance:
- ✅ A rendered readme and a rendered signature page report no non-embedded fonts, read out of the
  produced document. They carry no `/CIDSet` either.
- ✅ The readme still fits, measured against the real prose. *(The overflow refusal is exercised in
  both directions by `TestRenderReadmeRefusesAnOverflowingBody`, which predates this slice.)*
- ✅ `Append`ing them into a user document does not change that document's own fonts — asserted
  **both ways**: no font of the user's disappears, and none of nib's non-embedded fonts arrives.

#### P04.S02 — the `/CIDSet` that claims more than the font program has *(done 2026-09-11, v1.129.41)*
Scope: ua1 7.21.4.2 t2 on every embedded subset pdfcpu writes. The clause is conditional on the
stream's presence, and PDF/UA does not require one — so the honest answer is very likely to remove
it rather than to compute a correct one. **PDF/A-1 does require it**, so the boundary between the UA
path and `pdfa.go`/`pdfa_gs.go` is the whole of the care here. Refs exit criterion 3.

**pdfcpu's own comment is the diagnosis.** `font/fontDict.go:252` — *"CIDSet computes a CIDSet for
used glyphs"* — written unconditionally at `:357`, with no configuration to turn it off. The clause
wants every CID in the font PROGRAM. Measured on a one-line authored page: 162 bytes, **16 bits
set**.

**The population is TWO doors, not one.** The Markdown conversion and the OCR text layer are where
nib embeds a face of its own, and both failed the clause. An office conversion does not — its fonts
come from LibreOffice, which produces neither font-rule failure — and a `/CIDSet` in a document nib
merely rewrites is that file's business.

**PDF/A is measured, not argued.** `/CIDSet` is required for subset fonts by **PDF/A-1 only** and
PDF/A-2 dropped it; nib targets **2b** (`pdfa.go:49` writes `<pdfaid:part>2</pdfaid:part>`). A
de-CIDSet authored document goes through `PreparePDFA` and veraPDF calls it 2b-conformant.

**Cost, measured:** 22.4 ms → 23.9 ms for a Markdown conversion — 1.5 ms on a step that was already
22 ms.

Acceptance:
- ✅ An embedded-font document nib authors fails neither 7.21.4.1 t1 nor 7.21.4.2 t2. Measured: nib's
  Markdown output now fails **`5 t1`** (refused by decision), **`6.2 t1` · `7.1 t3` · `7.1 t11`**
  (P05's tree) and **`7.2 t33` · `7.2 t34`** (the language nib cannot determine, `/pending 471`) —
  and no font clause at all.
- ✅ The PDF/A path is measured to still carry whatever it requires.
  `TestDroppingCIDSetsKeepsPDFAConformance` runs the reference validator at `2b`.
- ➖ **`knownUA1Deltas` does not change**, and that is correct rather than a miss: the census drives
  `tagFates`, and `StampTextLayer` has no drive function there (it needs an OCR layer and its fonts)
  while `ConvertDocToPDF` is not an operation over a PDF at all. The oracle's population and this
  slice's do not overlap. **Covered instead by a stimulus floor that rots on purpose**:
  `TestPdfcpuStillWritesTheCIDSetTheDoorRemoves` goes red-as-a-skip when a later pdfcpu stops
  writing the stream, so the door gets retired rather than carried forever.

#### P04.S03 — the font install becomes load-bearing, and an unwritable `$HOME` must still work *(done 2026-09-11, v1.129.42)*
Scope: today a failed font install costs **non-Latin OCR only**, and `InstallOCRFonts` degrades
rather than blocking startup (the `fault.Catch` block at `ocrfonts.go:160-171`, written after a
read-only `$HOME` crashed nib at startup). After S01 it costs **every authored document's text**.
Refs D7.

**(grill, 2026-09-11 — confirmed, and the defect is OLDER than the slice that exposed it.)**
Measured with the pdfcpu user-font directory at mode 0500: every Markdown conversion returned
`install fallback font Roboto-Regular: permission denied` and produced nothing. **That was already
true of the fallback pool before the base faces existed** — `markdownFallbackFonts()` supplies
thirteen faces on every conversion, and `installFallbacks` returned the first failure — so P04 did
not introduce this. P04 is what made it visible.

**The fix turns on one distinction: whose fault is it.** An unwritable font directory is a
condition on the user's machine and must cost them a prettier document, never the document. A face
declared with the wrong name is a bug in nib that must not be papered over into silently-worse
output on every machine. `ErrFaceMisdeclared` is a sentinel rather than a message so the two can be
told apart — and it is the same error that already caught `LiberationMono-Regular` in S01.

**A silent degrade is indistinguishable from working**, so the notice is raised in `pdfops`, where
there is a logger. `mdpdf` deliberately has none: it lives at the repo root so other projects can
import it, and a logging dependency would travel with it. `mdpdf.InstallFaces` exists for exactly
this — installing an already-installed face is a map lookup, so asking twice costs nothing.

Acceptance:
- ✅ With the user font dir unwritable, authored output still renders — degraded to core fonts and
  saying so, never failing.
- ✅ The degrade is exercised, not reasoned: the test `chmod`s the directory to 0500, and **asserts
  an install into it actually fails before grading anything** — otherwise the whole test passes for
  the wrong reason. It skips loudly under `root`, which ignores the mode.
- ✅ A document that degraded is not claimed to embed its fonts. The message and the document are
  cross-checked against each other in one test: the log must say it degraded **and**
  `nonEmbeddedFonts` must agree, because a message nobody can check is worse than none. *(That
  cross-check arm is not independently probed: the state "the notice fired and the fonts embedded
  anyway" is unreachable — using a face whose install failed panics inside pdfcpu.)*

#### P04.S04 — a document nib did not author is refused for UA export *(closed 2026-09-11, v1.129.44 — NO CODE, moved to P07)*
Scope: `nonEmbeddedFonts()` already exists and `pdfaBlockers` already refuses-rather-than-mislabels;
this is the UA analogue. **There is no UA export door yet** — establish whether one is P04's or
waits for the tree, before building a refusal for an export nothing performs. Refs exit criterion 2.

**(grill, 2026-09-11 — the second acceptance clause is the one that fired, and it fired on a search
rather than on a judgment.)**

**There is no UA export anywhere.** Named search:
`grep -rniE "pdfua|pdf/ua|uaexport|prepareua" --include=*.go --include=*.js --include=*.html
internal/ web/ cmd/` returns **four hits, all in this phase's own test files**, plus `pdfuaid` in
the ADR-031 discussion. Nib has a PDF/A door (`PreparePDFA`, `ConvertPDFAGhostscript`, a route and a
CLI command); it has no PDF/UA door of any kind.

**So building the refusal now would be a remedy nobody can run.** Three reasons, and the first is
enforced by the tree:

- An exported `uaBlockers` with no caller goes **red at tier 1** — `zerocaller_test.go` exists
  because a crypto fix once landed in code nothing called (`/pending 441`, 445).
- A refusal is only correct relative to what the door promises, and there is no door to promise
  anything. *"Refused for UA export"* is meaningless until something exports.
- `~/.claude/CLAUDE.md`'s own rule: **a remedy is a claim, and it is the one nobody re-checks.**
  Writing it into a plan as a mandatory requirement is exactly how an untested suggestion becomes
  settled law.

**It belongs at P07**, whose exit criterion already says *"the report is reachable from the UI and
from the CLI"* — the first point at which nib has a UA surface a refusal can hang off, and the
point at which the checker can say **which** rules a document fails rather than only this one.

**Nothing is currently lying, which is why this can wait.** The only thing that would make nib
assert UA-ness about a document is ua1 `5 t1`'s `pdfuaid:part`, and P03.S01 **refused** to write it.
A document nib did not author is not claimed to be anything.

Acceptance:
- ➖ ~~The refusal names the fonts, the way `pdfaBlockers` names them.~~ **Not built.**
- ✅ *"or the slice closes with the finding that it is not yet reachable and says where it belongs"*
  — this clause, and it is the one the slice was written to allow.

### P05 — The tag tree core *(done 2026-09-11, v1.129.51)*
**Goal.** The typed model of D8 plus the wrapping emitter of D3 — parse, mutate, write back, with
`/ParentTree`, `/StructParents` and MCIDs as model invariants. This is the new capability the whole
plan rests on and the first place nib emits content-stream operators of its own.

**Exit criteria.** Round-trip of an existing tagged document is lossless; a tree built by the model
validates under veraPDF `ua1`; wrapping is proved not to disturb the wrapped content's bytes.

**Acceptance ledger — phase close, 2026-09-11 at v1.129.51.** Every clause split on `and`.

| # | clause | verdict |
|---|---|---|
| 1a | round-trip of an existing tagged document is **lossless** | ✅ **and it had NO READER until this close asked for one.** S02 proved the model PARSES faithfully — a different claim from writing back everything it parsed, and a model quietly dropping `/A` or an `OBJR` would have passed every S02 assertion. Now driven on the REAL tree: element types, page liveness, MCIDs, OBJRs and the role map compared across a no-op write |
| 1b | …of an **existing** document (not one nib authored) | ✅ a LibreOffice HTML conversion — role-mapped names, an `OBJR`, 13 integer MCIDs. The generated fixtures have none of the three and cannot stand in, so it skips loudly without LibreOffice |
| 2 | a tree built by the model **validates under veraPDF ua1** | ✅ `6.2 t1`, `7.1 t3` and `7.1 t11` clear **together**, measured before and after on the same document, with the untagged one asserted to fail all three first. Nib's Markdown output then fails **`5 t1` alone** |
| 3 | wrapping is proved **not to disturb the wrapped content's bytes** | ✅ a byte comparison of the span between the inserted operators, on a 2,093-byte real page — which is what S01's byte-identical round trip existed to make possible |

**Required-run gates, enumerated** (they are a separate list from the criteria and nothing else
walks them): the `CLAUDE.md` **slice gate does NOT fire** for any P05 slice — `git diff --name-only`
across the phase touches no `internal/server` session/ceremony/delivery/discovery path, no
`internal/p2p`, no `internal/rendezvous`. Tier 0 ✅, tier 1 ✅, tier 2 ✅ 343/343. Tier 3 not re-run:
the phase changed no `web/` file.

**What the phase found that no slice predicted:**

- **The round-trip law is not sufficient** — every tokenization covering the stream passes it, and
  three mutations proved it by staying green (S01).
- **`inspectTags` had undercounted since v1.129.16**, keying its visited set on dictionary content,
  so identical sibling elements counted as one (S02).
- **`/ParentTree` is two structures wearing one name** — a page's entry is an array indexed by MCID,
  an annotation's is a single reference (S03).
- **An element with no `/P` fails a clause that names the CONTENT**, so the symptom points away from
  the cause (S04).
- **A symmetric loss is invisible to a before/after comparison** — dropping the role map entirely
  left the round trip green (this close).
- **Three comments claimed more than their code could show**, each correct and each unreachable
  through its only caller (S01's `/ID`, S02's depth bound, S03's fill-versus-append).

**And the phase's own gate retired itself on schedule.** S01 added `gated` as a fifth zero-caller
exemption prefix with `TestNoGatedExemptionOutlivesItsCoordinate`; S04 shipped, four rows gained
production callers, and the guard failed the moment the slice was marked done — before the commit,
because the marker is written first. `TagAuthored` is now gated on **P06** for the same reason, so
"deliberately unwired" cannot become "forgotten".

**Shared surface.** `PLAN-text-reflow.md`'s P05 needs the same content-stream walker, for a harder
job (rewriting operators rather than bracketing them). Whichever plan reaches it first builds it and
the other extends it; built twice, the two will disagree about the same bytes.

**(phase-open, 2026-09-11 at v1.129.44 — four facts, and the first settles the shared surface.)**

- **This plan reaches the walker first, and the condition is checked rather than assumed.**
  `PLAN-text-reflow.md`'s P01 is done (v1.128.69) and its **P02–P05 are unstarted**, so nothing is
  waiting on a walker that exists. This phase builds it; text-reflow's P05 extends it.
- **Nothing in the tree parses a content stream, and neither does pdfcpu.** Named search across
  `internal/` for a walker, a tokenizer or an operator type: **zero**. In pdfcpu v0.13.0 there is no
  content-stream tokenizer and no text extraction at all — `ExtractContent` hands over decoded
  bytes and stops. The capability is genuinely new, which is what the phase goal says.
- **The read/write path is already proven in this repo.** `wrapPageToBox`
  (`pdfops.go:969-997`) does the whole round trip: `ctx.PageContent(d, pageNr)` for the decoded
  bytes, then `NewStreamDictForBuf` → `sd.Encode()` → `IndRefForNewObject` → `d["Contents"]`. The
  walker needs no new pdfcpu surface, only a parser between those two halves.
- **`/MarkInfo` is owed here by name.** P03 struck it from its own floor because setting
  `/Marked true` with no `/StructTreeRoot` is exactly `tagState.orphaned()` — what ADR-031 law 1
  forbids and P01.S06 built a door to prevent. It arrives with the tree.

**One question is deliberately NOT firmed into a slice: where `5 t1` belongs.** P03.S01 refused to
write `pdfuaid:part` because it was a conformance assertion over untagged content. A tagged document
makes it *possible* to be true — but not automatically true, since the document must actually
conform for the assertion to be honest, and the thing that can say so is **P07's checker**. Firming
a P05 slice for it would be deciding that question by scheduling. Recorded here; taken when P07's
exit criteria are firmed, or earlier if the tree turns out to carry the whole answer.

**Firmed slices:**

#### P05.S01 — the content-stream walker *(done 2026-09-11, v1.129.46)*
Scope: tokenize a decoded content stream into operand/operator steps and write it back. The surface
`PLAN-text-reflow.md`'s P05 extends. Refs D3, and that plan's P05 note.

**(grill, 2026-09-11 — confirmed, with one shape change the attack made structural and two
populations the plan did not have.)**

**A token is a SPAN, not a value, and that is what makes law 1 hold.** The plan writes
byte-identical round-trip as a property to test for; it is a property to achieve by construction. A
token carrying a parsed value makes identity a per-lexical-form battle nobody wins — `1.0` against
`1.` against `+1`, dictionary spacing, `#20` escapes inside names — and every one of those is a
silent corruption of somebody's document. A token carrying `(kind, start, end)` over the original
bytes makes identity free, and the round-trip test becomes a check that **nothing re-serialises**
rather than a check that everything re-serialises correctly.

**It is also what every later slice actually needs.** S04 brackets page content at offsets;
`PLAN-text-reflow.md`'s P05 replaces one run's bytes. Neither needs a decoded value — both need
boundaries and a splice. Values are decoded by whoever wants one, on the span.

**Inline images are not an edge case; they are the only construct that is not self-delimiting.**
After `ID` the bytes are raw binary whose length the content stream never states — derivable from
`/W /H /BPC /CS` only when no filter is applied, which is not the general case. The spec's own rule
is to scan for `EI` preceded by whitespace and sanity-check what follows. A tokenizer without that
special case reads image bytes as operators and corrupts them. **The corpus has none** (`BI` = 0,
measured on both fixtures), so this needs a built fixture.

**And the population the plan names predates the documents nib now produces.** Measured on nib's own
Markdown output at v1.129.45: **79 NUL bytes and a `\\` escape inside literal strings**, because
P04 made every glyph a two-byte index. Escape handling and paren balance are load-bearing on nib's
own files today, not on hypothetical ones — so the round-trip population is the corpus **plus nib's
current authored output**.

**It lives in `internal/contentstream`, not `internal/pdfops`.** It is the surface a second plan
extends, it depends on nothing of nib's, and `pdfops` is already the largest package in the tree.

Tasks:
- T01 — the tokenizer: kind + span, over the whole operand/operator grammar.
- T02 — inline images as their own token, with the `EI` rule.
- T03 — the writer: emit spans verbatim, and splice at a chosen token boundary.
- T04 — the round-trip law, over the corpus AND nib's own output.
- T05 — the cost, measured on a real page.

Acceptance:
- ✅ **Law 1: an unedited page round-trips BYTE-IDENTICALLY** across the corpus — 6 pages, 2,533
  bytes, including nib's own output. **AMENDED: the law is necessary and nowhere near sufficient**,
  and that was found by mutation rather than by reasoning. *Every* tokenization that covers the
  stream passes a round-trip test, including one emitting a token per byte — three separate
  mutations (the backslash-escape rule, paren nesting, the inline-image marker) left the whole
  round-trip suite green because every byte was still in some token and the writer still copied them
  in order. So the lexical rules are asserted as **shape**: `TestTheTOKENIZATIONIsRight` compares the
  token sequence, and each of its rows is a rule a mutation left undetected until it was written
  down.
- ✅ Inline images, strings and hex literals containing operator-looking bytes, dictionaries and
  nested marked content all survive — each a fixture, including **every byte value 0x00–0xFF inside
  one literal string**, and a truncation fixture that drives all 71 prefixes of a stream.
- ✅ The cost is **measured on a real page**: a 2,093-byte page from nib's own output is 858 tokens
  and tokenizes in **38 µs (55 MB/s, 12 allocations)** — against **1,549 µs** to read that page out
  of the document at all. A walk is **2.4%** of the cost of obtaining the bytes, so a later slice can
  walk every page without thinking about it. Both benchmarks ship, because a figure with no
  comparison is an adjective.

**The zero-caller guard fired, correctly, and gained a fifth prefix that expires.** The walker is
built at S01 and first *called* at S04, so for three slices it is exported code with no production
caller — which is a legitimate shape a plan produces, and also exactly what `/pending 442` punished
(dead exported crypto behind a standing exemption row that outlived its reason by months). The four
existing prefixes did not have a claim for it, so `gated — <plan> <coordinate>.` is the fifth, and
**`TestNoGatedExemptionOutlivesItsCoordinate` fails the day that coordinate is marked done**: either
the caller exists and the rows must go, or the slice shipped without building it and exported dead
code has just been released. The exemption retires itself rather than waiting to be noticed.

**`Write` had to be renamed `WriteTokens` to be checkable at all.** The zero-caller scan counts
identifier occurrences, so an exported function named `Write` is indistinguishable from every
`buf.Write` in the tree and could never be reported as uncalled — permanently exempt from the one
check that finds dead exported code, without anybody deciding that.

**One rule is honestly weaker than it looked, and it is recorded rather than dressed up.** The
`/ID`-is-not-the-marker fix reads correctly against the grammar, and reverting it leaves every test
green on well-formed input — because the image token is opaque, so believing the payload starts
earlier changes nothing unless a whitespace-delimited `EI` lies between, which a dictionary of names
and numbers cannot contain. It is driven on **malformed** input instead, and says so: a tokenizer's
job includes not making a broken stream worse.

#### P05.S02 — the structure tree as a typed model, read *(done 2026-09-11, v1.129.47)*
Scope: parse an existing `/StructTreeRoot` into Go — elements, `/K` children, `/S`, `/Pg`,
`/ParentTree` — with the LibreOffice corpus fixture (45 elements, 4 pages) as the reference. Refs D8.

**(grill, 2026-09-11 — confirmed, and it starts by finding a defect in the oracle its own acceptance
is written against.)**

**`inspectTags` undercounts, and it has since v1.129.16.** Its visited set is keyed on `d.String()`
— the dictionary's CONTENT — so two struct elements with byte-identical dictionaries are counted
**once**. Measured on a fixture with two identical `<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R >>`
siblings: `elements=1, anchored=1`. Real documents produce identical siblings easily (an empty
`/Span` repeated), and every figure this plan records from that oracle — the 45, the 3800/2247/1157
— is a lower bound rather than a count.

**Its blast radius is the COUNTS and not the verdicts**, which is why nothing caught it: every use
is comparative (`after.elements < before.elements`) or a zero test (`s.elements != 0`), and a
proportional undercount flips neither. That is also what makes it safe to fix inside this slice.

**So `inspectTags` is re-expressed on the model rather than left beside it.** D8 says every operation
reporting `carried` routes through one model; two traversals of the same tree, keyed differently, is
ADR-009's exact failure with the drift already measured.

**What a real tree actually contains, measured — because the plan's sketch lists three things and a
real one has six.** A LibreOffice-produced HTML conversion, 36 elements:

| | |
|---|---|
| root keys | `Type` `K` `ParentTree` **`RoleMap`** — and **no `/ParentTreeNextKey`**, so nothing may depend on it |
| element types | **15 distinct**, and four are role-mapped custom names: `Heading 1`, `Text body`, `Table Contents`, `Table Heading`, alongside `H2` `L` `LI` `Lbl` `LBody` `Table` `TR` `TH` `TD` `Link` `Document` |
| `/K` entry kinds | 36 indirect refs, **23 integer MCIDs**, and **one `OBJR` dict** — the `<a href>`'s annotation |
| element keys | `S` `P` `Pg` `K` `Type` on all 36, **`/A` attributes on 23** |

**`/RoleMap` is load-bearing, not decoration.** An element typed `Preformatted Text` means nothing
without it; a model that drops it loses what every non-standard element IS.

**And that is what makes the "refuse rather than partially parse" clause concrete.** The kinds a `/K`
entry may take are enumerable — integer, indirect element, direct element dict, `MCR`, `OBJR` — so
the model recognises each by name and **refuses an entry it cannot classify**, rather than skipping
it and reporting a tree with fewer children than the document has.

Tasks:
- T01 — the model: tree, element, the five `/K` entry kinds, `/RoleMap`, `/A`, `/Pg` resolved to a
  live page.
- T02 — refusal: an unclassifiable `/K` entry or a cyclic tree is an error, not a silent drop.
- T03 — `inspectTags` re-expressed on the model, and the undercount fixed.
- T04 — the population: the generated fixtures, the twin-element fixture, and a REAL role-mapped
  tree with `OBJR` and MCIDs in it.

Acceptance:
- ✅ Parsing an untouched tree and re-deriving the counts produces what the document actually
  contains. `inspectTags` is now the model, so the twin-element fixture reports **2** where it
  reported 1, and both are compared against a **from-scratch object-keyed walk** that shares no code
  with the model — a model checked against itself confirms itself.
- ✅ A tree nib cannot represent is refused rather than partially parsed, and the refusal names what
  it could not represent. **Driven through a NON-VALIDATING read, and that is a finding**: pdfcpu
  already rejects the document (`validateStructElementKArrayElement: invalid dictType Bookmark`), so
  the three kinds it permits are exactly the three the model represents and the refusal is
  unreachable through nib's normal door. It is a second line of defence, it is driven as one, and
  the test's floor asserts pdfcpu still rejects it — so the day that changes, somebody is told.
- ✅ The corpus is driven, and the population gained the real tree it needed. Measured on a
  LibreOffice HTML conversion: **21 elements, 14 distinct types** (`Heading 1`, `Text body`,
  `Table Contents`, `Table Heading` role-mapped beside `H2` `L` `LI` `Lbl` `LBody` `Table` `TR` `TH`
  `TD` `Link` `Document`), **4 role-map entries, 1 OBJR, 13 integer MCIDs** — none of which any
  generated fixture contains. It skips loudly without LibreOffice rather than passing.

**Two fixtures exist because mutation found the tests could not tell.** Disabling the visited set
left every test green — the generated corpus is all simple trees — so `sharedElementFixture` reaches
one element from two parents, which is ordinary and legal, and counts **4 where the document has 3**
without it. `cyclicElementFixture` drives termination on a two-element cycle.

**And the depth bound is declared as untested rather than implied tested.** Raising
`maxStructDepth` to 100,000 leaves everything green, because the visited set stops every cycle that
can actually be built — a cycle needs an element reached twice, which needs an indirect reference,
which the set catches. It stays because the failure it guards is unrecoverable and the set is one
edit away from being weakened.

#### P05.S03 — `/ParentTree`, `/StructParents` and MCIDs as model invariants *(done 2026-09-11, v1.129.48)*
Scope: the write half, with the three things every caller currently has to remember maintained by
the model instead. Refs D8, D9.

**(grill, 2026-09-11 — confirmed, and `/ParentTree` turns out to be two structures wearing one
name.)** Measured on a real LibreOffice tree:

	key 0 -> an ARRAY of 23 element references   ← a PAGE's entry, indexed BY MCID
	key 1 -> a single element reference          ← an ANNOTATION's entry

A page carries `/StructParents` (**plural**) and its entry is an array *whose index is the MCID*;
an annotation or Form XObject carries `/StructParent` (**singular**) and its entry is one reference.
The two spellings differ by one letter and mean different shapes, and a model treating the tree as
"key → element" destroys the first.

**The checker was built before the writer, and pointed at documents that already exist.** A
post-condition nobody can evaluate is a post-condition nobody has — and running it over the corpus,
a real tree, and **`NUp`'s output** asks P01.S06's work a question P01 never had an instrument for.
Result: **0 defects everywhere**, which is not a null result — it is independent corroboration that
`carryTagsThroughNUp` produces a self-consistent tree, by something other than a veraPDF clause
count.

Tasks:
- T01 — the four invariants, as a checker over any document.
- T02 — run it over the corpus, a real tree, `NUp` and `Rotate`.
- T03 — `addMarkedElement`: the one mutation, with the checker as its post-condition.
- T04 — D9's page-subset remap: measured or named unmeasured.

Acceptance:
- ✅ **ADD** leaves `/ParentTree` and every page's `/StructParents` consistent, checked by reading
  them back. ~~Removing and re-parenting.~~ **NOT BUILT, and that is the recorded decision rather
  than an omission**: neither has a caller — P05.S04 wraps content, which adds — and an untested
  mutation of a structure tree is `/pending 442`'s bet in a worse form, because its failures are
  silent. A tree that still parses and describes the wrong content looks exactly like a correct one.
- ✅ D9's page-subset remap is **named as still unmeasured**, which is the clause's own second
  option. It needs `Collect`/`RemovePages` to rebuild a tree for a kept subset, and this slice built
  the invariant checker that such a remap would have to satisfy — which is the prerequisite, not the
  thing. Nothing in P05 needs it; **P06 is where an operation would.**
- ➖ `TestEveryDeclaredFateIsTheMEASUREDFate` re-run: **no verdict changes.** The model makes a remap
  *possible*; it does not make any operation carry a tree it was not already carrying, so every row
  stays as measured. Re-declaring one on the strength of a capability nothing uses would be exactly
  the false declaration law 2 exists to catch.

**Five mutations red, and one of them exposed a claim the code could not support.** The ParentTree
array is indexed by MCID, so growing it must FILL rather than append — and `addMarkedElement`
allocates `mcid` as the array's length, so the two coincide on every call it makes. Swapping fill
for append left every test green. The fill is `setParentTreeSlot`'s general contract, which P05.S04
needs when it wraps content whose MCIDs already exist, so it is now driven directly instead of being
an untested sentence in a comment.

#### P05.S04 — the wrapping emitter *(done 2026-09-11, v1.129.49)*
Scope: bracket page content in `BDC`/`EMC` with MCIDs the model knows about. The first place nib
emits content-stream operators of its own. Refs D3.

**(grill, 2026-09-11 — confirmed, and the slice's real decision is WHAT an emitter that knows no
semantics may claim.)** It knows where a page's content is and nothing about what it says:

- **`/P` would say every page is one paragraph**, which is false of any page with a heading on it.
  That is ADR-031 law 1's species in a quieter register — a claim about content nib has not
  examined.
- **`/Div`** is ISO 32000-1's generic block-level grouping element. It says *this is real content,
  grouped*, which is the whole of what this code knows.

Real structure — headings as `H1`, lists as `L`/`LI`, from `mdpdf`'s own AST — is **P06**, which has
the information this does not. P06 replaces the TYPE, not the mechanism.

Tasks:
- T01 — the emitter: wrap each unmarked page, register the element, report how many it wrapped.
- T02 — `ensureStructTree`: create an empty tree where a document has none, refuse one it cannot
  represent.
- T03 — idempotence: a page already carrying marked content is left alone, decided by TOKENS.

Acceptance:
- ✅ **Wrapping does not disturb the wrapped content's bytes** — asserted as a byte comparison of
  the span between the inserted operators, on a 2,093-byte real page. This is what S01's
  byte-identical round trip was for: the emitter inserts at two offsets and copies everything else.
- ✅ The MCIDs the emitter writes are the ones the model's `/ParentTree` points at, **read back out
  of the produced document** rather than remembered from the call — the two directions live in
  different places and nothing but a check makes them the same number.
- ✅ A page that is already marked is not double-wrapped, and **the check is TOKENIZED**: since P04
  every glyph nib draws is a two-byte index, so a page whose text happens to contain the bytes `BDC`
  would be skipped by a byte search and come out untagged with nothing saying why.

**The slice's defect was found by measurement and it looks like something else.** The emitted
element had no **`/P`**, and the symptom is that veraPDF reports every content item as `{mcid:0}` —
marked, so the bracketing worked — while failing ua1 **7.1 t3**, *content shall be marked as
Artifact or tagged as real content*. **The failure names the CONTENT, so it reads as a wrapping
problem and is not one**: an element outside the tree is not something an MCID can resolve into.
`/P` is required by ISO 32000-1 table 323 and every element of the real LibreOffice tree carried
one — 36 of 36, measured at S02 and not read from the specification.

**Measured end to end, with S05's `/MarkInfo` applied as a probe:** nib's Markdown output fails
**`5 t1` and nothing else** — the conformance assertion nib refuses by decision. `6.2 t1`, `7.1 t3`,
`7.1 t11`, `7.2 t33` and `7.2 t34` all clear.

**And P05.S01's `gated` exemptions retired themselves in this slice**, which is the mechanism
working on the first coordinate it was written for. S01 gated five zero-caller rows on `P05.S04`;
this slice gave four of them production callers, and the guard failed **the moment the slice was
marked done** — before the commit, because `/createcode` writes the marker first. The two survivors
became `test-support`, since nothing schedules a caller for them and a coordinate would be a date
nobody is keeping.

#### P05.S05 — `/MarkInfo`, and a document that is honestly tagged *(done 2026-09-11, v1.129.50)*
Scope: the clause P03 deferred here by name, plus the end-to-end proof. Refs exit criterion 2,
ADR-031 law 1.

**(grill, 2026-09-11 — confirmed, and step zero raised a question the ADR had already answered.)**

**`TagAuthored` is built and deliberately NOT WIRED into the authoring doors.** The obvious next
move is to tag every document nib writes. ADR-031 says why not, in its own words: *"A screen reader
told a document is tagged **stops reaching for the fallbacks it would otherwise use**. A user who
can see their tagging is gone can re-tag, re-export, or choose a different tool; a user handed a
document that asserts structure it does not have has had their own check defeated."*

A `/Div`-per-page tree asserts structure while carrying **none of the distinctions a reader
navigates by** — no headings, no lists, no paragraph boundaries. It clears the clauses and may hand
a user less than the untagged document would have, because the reader's own heuristics stop running.
That is the asymmetry law 1 is built on, applied one level up: the claim is true, and it is still
not worth making until it carries something.

**P06 is where it gets wired**, with real structure from `mdpdf`'s own AST — which is that phase's
stated goal and the thing that makes the claim worth making. The mechanism is finished here; what it
is pointed at is P06's.

Tasks:
- T01 — `TagAuthored`: both halves in one operation, with `orphaned()` as its own post-condition.
- T02 — the end-to-end ua1 measurement, before and after on the same document.

Acceptance:
- ✅ A nib-authored document gains `/MarkInfo /Marked true` **and** a populated `/StructTreeRoot` in
  one operation — **there is no parameter for doing one without the other**, because a caller that
  could eventually would. A document with nothing to wrap comes back byte-identical **and without
  `/MarkInfo`**: asserting tagging over a tree with no elements is the violation this exists to
  avoid, not a harmless extra key.
- ✅ `inspectTags` reports it un-orphaned, **and the predicate is proved able to see the failure** —
  `TestSplittingTheTwoHalvesIsOrphaned` builds the `/MarkInfo`-with-no-tree document and requires
  `orphaned()` to return true, so the clean result above is not a predicate that never fires.
- ✅ Measured on the ua1 oracle, before and after on the same document: **`6.2 t1`, `7.1 t3` and
  `7.1 t11` clear together**, nothing new arrives, and the untagged document is asserted to fail all
  three first — clearing a clause that already passed proves nothing. Nib's Markdown output, titled
  and with `/Lang`, then fails **`5 t1` alone**.
- ➖ The ua1 delta table does not change, and that is correct: `knownUA1Deltas` is keyed on
  `tagFates`, and `TagAuthored` is not a census operation — it takes a document and returns one, but
  nothing in the census drives it because it is not yet reachable from any authoring door.

### P06 — Tagging what nib authors *(done 2026-09-12, v1.129.61)*
**Goal.** Exact structure first (D4): `mdpdf` from its AST, then authored form fields with `/TU`
names and `/Tabs`, then OCR from tesseract's block/paragraph/line refs recovered across the wire.

**Exit criteria.** ~~A markdown document, an authored form, and an OCR'd scan each pass veraPDF
`ua1`; each tree records which of D4's three sources produced it.~~

**AMENDED at the phase close, 2026-09-12 at v1.129.60, after measuring all three.** The first
clause is unachievable by this phase or any other, for the same reason P01 struck its own: *"passes
`ua1`"* includes clauses about content nib did not author, about document metadata nobody supplied,
and about the identification nib deliberately refuses. A criterion no phase can meet is one every
phase closes over.

Measured, each document through its product pipeline:

| document | fails |
|---|---|
| a tagged Markdown document | `5 t1` — and nothing else |
| a described authored form | `7.1 t3` `7.1 t8` `7.1 t10` `7.21.4.1 t1` `7.21.7 t1` |
| a tagged OCR'd scan | `7.1 t3` `7.1 t8` `7.1 t10` |

Every clause in that residue belongs to one of four buckets, and **none of them is P06's**:

- **deliberately refused** — `5 t1`, the PDF/UA identification. P03.S01's decision; writing it needs
  something that can say the document conforms, which is **P07**.
- **content nib did not author** — `7.1 t3`, the host page a form was placed onto and the scan image
  itself. Tagging someone else's page content is P08's.
- **document metadata nobody supplied** — `7.1 t8` and `7.1 t10`. `SetTitle` exists and clears both;
  neither route calls it, because neither is given a title. Recorded rather than fixed here: writing
  metadata into the user's document is the same class of decision as `/pending 471`.
- **fonts** — `7.21.4.1 t1` and `7.21.7 t1` on the form alone. **D7 already decided this and
  assigned it to P04, which closed without covering the form door**: measured, `AuthorForm` embeds
  ZERO fonts where Markdown embeds 2 and the OCR layer 1. Filed as `/pending 479`.

**So the criteria are, in the form that is assertable and that a standing reader checks:**

1. A tagged Markdown document fails **no `ua1` clause except `5 t1`** — `TestATaggedMarkdownDocumentPassesUA1`.
2. A described authored form **fails no clause the undescribed one did not**, and clears `7.18.4 t1`,
   `7.1 t11` and `6.2 t1` — `TestTheDescribedFormClearsTheClauseS05CouldNot`.
3. A tagged OCR'd scan **fails a strict subset** of what the untagged scan failed —
   `TestTaggingAnOCRdScanCostsItNoUA1Clause`.
4. Every tree nib writes records which of D4's sources produced it —
   `TestEveryTreeNibWritesRecordsItsSource`, plus the per-door readers.

Clauses 2 and 3 are the P01 shape ("adds nothing not written down", checked both ways) rather than a
pass/fail count, because a clause count scores honesty as a regression — which P01 wrote down and
this criterion had forgotten.

**Carried in from P03.S02 — the one real language defect nib creates, and no catalog key can fix
it.** `AppendReadme` staples nib's own English prose into the user's document, and `pdfops.Append`
keeps the FIRST document's catalog: from that moment nib's English text is declared to be in
whatever language the user's document says, which for a German contract is German. The readme and
the signature pages are `CreateFromJSON` output rather than `mdpdf`'s, so they are a fourth source
alongside D4's three — small, entirely nib's own words, and the only one where the language is
known with certainty. The fix is a language on the CONTENT (a `/Span` carrying `/Lang`, or the
structure element's own `/Lang` attribute), which is P05's wrapping emitter applied here. P03.S02
deliberately did not do it twice.

**(phase-open, 2026-09-11 at v1.129.51 — five measurements, and two change what the phase contains.)**

| measured | result |
|---|---|
| what P05 left ready | `TagAuthored` wraps a page and builds a tree; **P06 replaces the TYPE, not the mechanism**. `addMarkedElement` takes a structure type and allocates the MCID; what it does not yet take is a PARENT, so a nested tree needs one more mutation |
| `mdpdf`'s structure | it walks a real goldmark AST (`renderer.blocks`) and knows heading level, list nesting and code blocks at layout time — **and throws all of it away**: `layout` records positioned runs and nothing about what produced them |
| the form third | **`/TU` and `/Tabs` are written NOWHERE** — named search across `internal/` returns zero. `AuthorForm` builds fields through a `CreateFromJSON` spec; neither key has ever been emitted |
| the OCR third | tesseract's hierarchy **is available and never crosses the wire**. `web/app.js:6587` iterates `data.words` and sends `{page, text, rect}`; the vendored tesseract.js exposes `blocks → paragraphs → lines → words` (`blocks:!0`, `t.blocks.forEach`, `paragraphs:`, `lines:` all present in the bundle) |
| the fourth source | the readme and signature pages are `CreateFromJSON` output — nib's own English prose, appended into a document whose `/Lang` may say otherwise. **The only source whose language is known with certainty** |

**So the OCR third is a WIRE CHANGE before it is a tagging change**, and that is the thing the
sketch's phrase *"recovered across the wire"* assumes rather than states. A new request field also
meets `/pending 447`'s guard, which reads `r.FormValue`/JSON fields as `(route, field)` pairs and
requires every field a handler reads to be one some client sends.

**And `mdpdf` has the information but discards it at the layout boundary**, which is where the work
is: `style` carries a font and a size, not a role. Teaching the layout to carry structure is a change
to the package `PLAN-text-reflow.md` also builds on, so it is cut as its own slice.

**Firmed slices:**

#### P06.S01 — `mdpdf` carries its own structure to the layout *(done 2026-09-11, v1.129.53)*
Scope: the renderer knows heading level, list nesting and code blocks from the goldmark AST and
throws them away at `layout.para`/`layout.code`. Carry them, as data on the run, without changing a
single byte of what is drawn. Refs D4's observed-exact tier.

**(grill, 2026-09-11 — confirmed, and the byte-identity clause had to be corrected before it could
be met by any implementation.)**

**Roles are POSITIONAL, not a tree.** `spec()` walks `l.runs` in order, one pdfcpu text entry per
run, and pdfcpu emits them in that order — so the Nth run is the Nth text-drawing operator in the
page's content stream. A caller that wants to bracket a run needs to know *which* run, and a flat
per-page list in draw order says exactly that. A tree would have to be re-flattened to be usable,
and the flattening is where a mismatch would hide. P06.S02 checks that correspondence rather than
assuming it.

Tasks:
- T01 — `Role`/`Structure`, and the role on the run.
- T02 — the renderer sets it as it walks: heading level, list depth, quote depth, code, marker.
- T03 — `ConvertStructured`, sharing ONE body with `ConvertWithFaces`.

Acceptance:
- ✅ Every laid-out run records which AST node produced it, **with the level and the depth** — the
  two things a flat *"this is a heading"* would lose and the two PDF/UA needs. Asserted against the
  SOURCE Markdown rather than a golden file of whatever the code produced.
- ✅ ~~The rendered bytes do not change, asserted byte-for-byte.~~ **AMENDED, because the original
  clause is unsatisfiable by any implementation**: pdfcpu writes a random `/ID` into every trailer,
  so the same function called twice on the same input already produces different bytes (measured —
  952 bytes both times, differing at offset 713). The clause now compares the **decoded content
  streams**, which is what *"nothing drawn changed"* actually means and has none of the trailer's
  randomness. Green over a document with headings, nested lists, ordered lists, nested quotes and a
  code block.
- ✅ `ConvertWithFaces` is untouched for a caller that asks for no structure — **it and
  `ConvertStructured` share one body**, because two renderer walks could disagree about what they
  produced and a structure describing a document drawn by different code is worse than none.

**Four mutations red, one not.** Flattening heading levels, flattening list depth, dropping the role
from the run, and dropping the marker path's save/restore all fail. `withRole`'s restore does not:
removing it leaves every test green, because every arm of `block` that lays out a RUN sets its own
role first, and `ThematicBreak` — the one that does not — emits a *box*, which carries no role. It
stays as a guard against the arm somebody adds without reading the comment, and the comment says it
is unreached rather than implying it is covered.

#### P06.S02 — a Markdown document is tagged from its AST *(done 2026-09-11, v1.129.54)*
Scope: `TagAuthored`'s mechanism pointed at S01's structure — `H1`–`H6`, `P`, `L`/`LI`/`LBody`,
`Code` — instead of one `/Div` per page. Needs `addMarkedElement` to take a parent. Refs exit
criterion 1, D4.

**(grill, 2026-09-11 — confirmed, and step zero found a gap in P06.S01's own shape.)**

**`{Kind, Level}` could not express what a tagger needs, and a tagged pin amended S01.** Measured on
a document with two consecutive paragraphs and a two-line code block: runs 1 and 2 were both
`body/0`, runs 7 and 8 were both `code/0` — and the two cases need **opposite** treatment. Two
paragraphs are two elements; two lines of one code block are two MCIDs of ONE. `Role.Block` is the
ordinal that tells them apart, and `layout.beginBlock` is the single door that assigns it so no
entry point can forget.

**The correspondence holds and is now checked rather than assumed**, which is what S01's doc comment
promised this slice would do: measured, a nine-run document draws nine top-level `q … Q` groups, and
`tagOnePage` **refuses** when the counts disagree. A silent mismatch attaches every element from the
point of divergence to the wrong content, and the document looks entirely correct.

**Marked content brackets the GROUP, not the `Tj`.** The font, the colour and the text matrix that
place a glyph are in the same `q … Q` group and belong inside the same marked-content sequence;
bracketing the `Tj` alone describes an extent that is legal and wrong.

**`/Code` is an INLINE type and a code block is not one**, so a fenced block is `/P` — *a block of
text*, which is true. A richer mapping (`/P` holding `/Code` spans, or a custom type role-mapped as
LibreOffice's `Preformatted Text` is) belongs with P09's structure editor.

Tasks:
- T01 — the S01 amendment: `Role.Block`, assigned in one place.
- T02 — `addMarkedElementUnder` and `addGroupingElement`: a parent, and elements that own no content.
- T03 — `addMCIDTo`: an element owning several MCIDs, for a wrapped paragraph.
- T04 — `textOperatorSpans` and `tagMarkdown`, with the correspondence refused when it breaks.

Acceptance:
- ✅ A Markdown document with headings, lists and a code block passes veraPDF `ua1` **except
  `5 t1`** — measured, and `5 t1` is the only clause left.
- ✅ The tree's shape matches the source, **checked against the SOURCE**: one `#` → one `H1`, one
  `##` → one `H2`, one list → one `L` with two `LI`, two paragraphs and one fence → three `P`. A
  golden file would say whatever the code produced on the day it was written.
- ✅ Nesting is real, read back from the written document: `L` → `LI` → `Lbl` + `LBody`, with every
  `/LI` required to hold exactly one of each. A flat tree of siblings satisfies every COUNT and
  describes a document with no list in it.
- ✅ **And a wrapped paragraph is ONE element with several MCIDs** — the clause S01's amendment
  exists for. Four runs of one paragraph must not become four paragraphs.

**A fourth font door was found by veraPDF, not by reading.** `tagMarkdown` reaches `mdpdf` directly
rather than through `ConvertDocToPDF`, so P04.S02's `/CIDSet` tail did not run and the TAGGED
document failed ua1 `7.21.4.2 t2` — a clause the untagged one passes. Fixed, and
`TestNothingNibEmbedsAFontIntoCarriesACIDSet` now drives this door too, which is exactly what its
own comment said it was for: *a list of call sites passes when a new door is added and not routed*.

#### P06.S03 — the tree records which source produced it *(done 2026-09-11, v1.129.55)*
Scope: D4's rank made visible — *"the user is told which of the three produced the tree they are
looking at"*. Refs exit criterion 2, D4.

**(grill, 2026-09-11 — confirmed, and D4's three tiers turn out to need a fourth.)**

**`/StructTreeRoot` has six defined entries and none says where the tree came from** — `Type`, `K`,
`IDTree`, `ParentTree`, `ParentTreeNextKey`, `RoleMap`, `ClassMap`. So this is a private key,
`/NibStructureSource`, named for nib so it cannot collide with a producer's own. **Measured: it
costs nothing** — pdfcpu validates the document and veraPDF raises no clause.

**It goes on the tree root, not in the XMP**, because it is a fact about the TREE and the tree root
is where a reader of the tree already is. Metadata is where a cataloguer looks; this is for whoever
is deciding how far to trust the structure in front of them.

**D4 names three tiers and a fourth was needed.** P05.S04's emitter brackets a page's content
knowing nothing about what it says — that is not *exact* (it read no AST), not *approximate* (it
derived nothing), and not *inferred* (it guessed nothing). `Generic` is the honest name, and the
alternative was calling that tree `Exact`, which would be false about the only thing this key exists
to say.

Tasks:
- T01 — the four tiers and the key.
- T02 — both writing doors record theirs.
- T03 — `StructureSource`, with **three** answers rather than two.

Acceptance:
- ✅ Every tree nib writes records its source — both doors today, `tagMarkdown` as `Exact` and
  `TagAuthored` as `Generic` — and the value **survives** `SetTitle`, `SetLang`, `Rotate` and a
  no-op rewrite. A record pdfcpu drops on the next write is a record that lasts until the first
  thing happens to the file.
- ✅ The sources are distinguishable **by reading the document**, not by trusting the writer.
- ✅ An unrecorded tree is not treated as exact. **This is the clause that matters**: every tree in
  the field today carries no record, so a reader defaulting to the best tier would describe all of
  them as the most trustworthy kind — ADR-031's law 1 in a new field.
- ✅ **And a foreign value is not read as a tier.** Another producer's private key that happens to
  share this name says nothing about D4's tiers, and guessing what it meant is worse than reporting
  nothing.

#### P06.S04 — nib's own prose declares its own language *(done 2026-09-11, v1.129.56)*
Scope: the defect P03.S02 measured and carried here. `AppendReadme` staples English into a document
whose `/Lang` may say otherwise; the fix is a language on the CONTENT, which P05's emitter makes
possible. Refs P03.S02, `/pending 471`.
Acceptance:
- A readme appended to a `/Lang=de` document declares its own text English, read back from the
  composed document.
- The user's own pages are unaffected — their language is not restated, overridden, or removed.
- Measured on the ua1 oracle: the composed document gains no clause.

#### P06.S05 — authored form fields carry `/TU` and `/Tabs` *(done 2026-09-11, v1.129.57)*
Scope: **neither key is written anywhere today** (named search: `grep -rn '"TU"\|/TU\b' internal/ web/`
finds one test COMMENT and no code; `/Tabs` zero hits). `/TU` is the accessible name a screen reader
announces for a field; `/Tabs /S` makes tab order follow the structure tree. Refs exit criterion 1,
D4's observed-exact tier, `/pending 471`.

**(grill, 2026-09-11 — AMENDED. The third acceptance clause was wrong and is replaced; step zero
measured all four combinations on the ua1 oracle before a line was written.)**

| document | ua1 form/language clauses failed |
|---|---|
| `AuthorForm` today | `7.18.1 t3` · `7.18.3 t1` · `7.18.4 t1` · `7.2 t34` |
| `+ /TU` | `7.18.3 t1` · `7.18.4 t1` · **`7.2 t25`** · `7.2 t34` |
| `+ /Tabs /S` | `7.18.1 t3` · `7.18.4 t1` · `7.2 t34` |
| `+ both, catalog `/Lang`` | `7.18.4 t1` |

Three findings reshape the slice:

- **`7.18.4 t1` is OUT OF REACH here and the old clause claimed it closes.** veraPDF's words: *"A
  Widget annotation shall be nested within a Form tag"*, failing with *"nested within null tag
  (standard type = null) instead of Form"*. That is a structure element with an `OBJR` kid — the
  emitter's work (P05.S03 already models `kidOBJR`), not a dictionary key. No combination of `/TU`
  and `/Tabs` moves it.
- **`/TU` ADDS `7.2 t25`** — *"Natural language in the TU key for form fields shall be determined"*.
  An accessible name is text, and text needs a language. Cleared ONLY by the catalog `/Lang`;
  `/Lang` on the field dictionary itself does nothing (measured, with the mutation confirmed to have
  landed). `AuthorForm` works on the USER'S document, so writing that catalog key is the guess
  P03.S02 measured and `/pending 471` parks — **so the slice writes `/TU` anyway and records
  `7.2 t25` against the no-`/Lang` case with 471 as its gate.** Withholding an accessible name to
  keep a clause table clean is scoring honesty as a regression, which P01 already wrote down.
- **The label the old clause requires does not exist.** `web/app.js:6910` has exactly one user
  string — `f.input.value` — which it trims, defaults to `field_N`, and de-dupes with a numeric
  suffix before it becomes `/T`. `FormField` gains a `Label` carrying the RAW typed value, or `/TU`
  is `/T` spelled twice. Where the user typed nothing there is no name to give: `/TU` is omitted,
  because `field_3` is identical to the `/T` a reader already falls back to and is the generic
  label `TitleFromName` refuses on the same reasoning.

Acceptance:
- Every field `AuthorForm` places **for which the user gave a name** carries a `/TU`, and it is that
  name — not the de-duped internal field name, which is what `/T` already is. A field the user did
  not name carries no `/TU`, and that is asserted rather than left to happen.
- Every page carrying a widget carries `/Tabs /S`, through one door.
- `knownUA1Deltas["AuthorForm"]` **shrinks from three clauses to one**: `7.18.1 t3` and `7.18.3 t1`
  go; `7.18.4 t1` stays and its row says it needs a `Form` structure element, naming the slice that
  can close it.
- On a document with NO catalog `/Lang`, `/TU` adds `7.2 t25` and nothing else — asserted, with
  `/pending 471` named as the gate, so the cost of the decision is measured rather than described.

#### P06.S06 — an OCR'd scan is tagged from tesseract's own hierarchy *(done 2026-09-12, v1.129.59)*
Scope: the wire carries block/paragraph/line, and the text layer is tagged from it. **A request-field
change**, so it meets `/pending 447`'s guard. Refs exit criterion 1, D4's observed-approximate tier.

**(grill, 2026-09-11 — AMENDED, and the slice's subject changed. Five step-zero measurements.)**

- **The OCR layer is not "untagged". It is marked `/Artifact`, which says it is NOT REAL CONTENT.**
  Measured on the stamped page stream: every word is wrapped
  `/Artifact <</Subtype /Watermark /Type /Pagination>> BDC … EMC`, because `StampTextLayer` goes
  through `api.TextWatermark` and pdfcpu artifacts a watermark by construction. PDF/UA's whole point
  about an artifact is that conforming readers skip it. So nib's searchable text layer — the one
  thing that makes a scan readable at all — is explicitly declared not to be read, and the document
  satisfies ua1 7.1 t3 for that content by DISCLAIMING it rather than describing it. **The work is
  replacing a disclaimer, not adding structure to bare content.**
- **Each word is its own Form XObject**, invoked `/Fm0 Do` from the page stream inside that artifact
  bracket — the text is not in the page stream. That is good news: the MCID goes in the PAGE stream
  around the `Do`, which is what P05's `contentstream` editor already does, rather than needing
  `/StmOwn` refs into each XObject.
- **The hierarchy needs no restructuring of the OCR call.** tesseract.js 5.1.1 (vendored) walks
  `blocks → paragraphs → lines → words → symbols` and returns the flattened arrays with BACK-POINTERS
  on every entry: a word carries `{page, block, paragraph, line}`. So the client sends three integer
  indices per word, not a nested tree — and the objects themselves must NOT be sent, being large and
  mutually referential.
- **`/pending 473` was a hard blocker and is now closed** (v1.129.58). Before it, `StampTextLayer`
  added ua1 `7.10 t1` and `7.10 t2` to every document — measured at this slice's step zero, which is
  how the OCR door was found missing from the census entirely.
- **PDF/UA has no line-level structure type.** `line` orders words within a paragraph and becomes no
  element. The mapping is block → `Sect`, paragraph → `P`, word → an MCID inside that `P`.

Acceptance:
- The client sends the hierarchy it already has — three integers per word — with a reader that goes
  RED when it stops. **`/pending 447`'s guard is not that reader and the sketch was wrong to name
  it**: `fieldsRead` collects only `r.FormValue`, `PostFormValue` and `Query().Get` string literals,
  so it covers form and query carriers and no JSON body at all. `/api/ocr` is JSON, so neither
  `lang` nor `words` was ever in its population — probed: deleting the client's `block`/`para`/`line`
  leaves it green. Filed as `/pending 477`; the reader here is a tier-2 source scan, as at S05.
- **No word of the text layer is marked `/Artifact` any more**, and that is asserted directly on the
  page stream rather than inferred from a clause.
- An OCR'd scan passes veraPDF `ua1` except `5 t1` and anything its images owe, with what it still
  owes named per clause rather than summarised.
- The tree is marked **OCR-derived** per D4 and S03 — `sourceApproximate` is written and recorded,
  never passed off as exact.


#### P06.S07 — the authored form gets a structure tree *(done 2026-09-12, v1.129.60)*
**(added at the phase close, 2026-09-12 at v1.129.59 — the phase's own exit criteria were measured
and TWO of them were unmet by the form. This is P01's sixth slice and P02's fifth, again: a
criterion met by building the thing, not by striking the clause.)**

Scope: `AuthorForm` writes `/TU` and `/Tabs` (S05) and **no tree at all**. So the phase's exit
criteria fail twice over on the form — it does not pass `ua1`, and there is no tree to record a
source for. Refs exit criteria, D4's observed-exact tier, S05's amendment, P05.S03's `kidOBJR`.

Measured before firming, on veraPDF ua1:

| document | clauses failed |
|---|---|
| authored form, no tree | `6.2 t1` · `7.1 t10` · `7.1 t11` · `7.1 t3` · `7.1 t8` · **`7.18.4 t1`** · `7.21.4.1 t1` |
| \+ a `/Form` element per widget with an `OBJR` kid | `6.2 t1` · `7.1 t10` · `7.1 t3` · `7.1 t8` · `7.21.4.1 t1` |

So **`7.18.4 t1` is reachable after all** — S05 proved only that no dictionary KEY reaches it.
veraPDF's demand is literal: *"A Widget annotation shall be nested within a Form tag"*. Three writes
do it, and `7.1 t11` clears with them:

- a `/Form` grouping element per widget, holding `{/Type /OBJR, /Obj <widget>, /Pg <page>}`;
- `/StructParent` (SINGULAR) on the annotation;
- a ParentTree entry that is **a single reference, not an array** — the two shapes P05.S03 modelled
  and which nothing has yet written. `setParentTreeSlot` fills an array slot and is the wrong door.

Acceptance:
- Every widget `AuthorForm` places is nested in a `/Form` structure element that points at it by
  `OBJR`, and the annotation points back by `/StructParent`.
- The ParentTree entry for an annotation is a single reference; `checkStructConsistency` sees no
  new defect, and a page's array entry is not confused with it.
- The product route authors through the new door, and **the census records the tagged door as adding
  NOTHING** — `7.18.4 t1` was `AuthorForm`'s last row and the tagged door clears it. `AuthorForm`
  keeps its row: it stays the untagged primitive `AuthorTaggedForm` builds on, the same shape
  `StampTextLayer`/`TagOCRLayer` already have, and a row saying an untagged form is untagged is
  true. The table's both-directions check is the reader for both halves.
- The tree records `sourceExact`: the widget-to-field correspondence is nib's own authored input,
  not a reading of a picture.
- `/MarkInfo` and the tree are written together or neither, and `orphaned()` is the post-condition —
  the law `TagAuthored`, `tagMarkdown` and `TagOCRLayer` all hold.


**Phase close — 2026-09-12, v1.129.61. Seven slices (S01–S07), the last added AT the close because
the criteria were measured and two of them were unmet.**

**Acceptance ledger**, against the amended criteria above, clause by clause:

| # | clause | verdict | evidence |
|---|---|---|---|
| 1 | a tagged Markdown document fails no `ua1` clause except `5 t1` | **met** | `TestATaggedMarkdownDocumentPassesUA1`: *"fails only: 5 t1 (refused by decision)"* |
| 2 | a described form fails no clause the undescribed one did not, and clears `7.18.4 t1` / `7.1 t11` / `6.2 t1` | **met** | `TestTheDescribedFormClearsTheClauseS05CouldNot`: undescribed `[6.2 t1 7.1 t10 7.1 t11 7.1 t3 7.1 t8 7.18.4 t1 7.21.4.1 t1 7.21.7 t1]` → described `[7.1 t10 7.1 t3 7.1 t8 7.21.4.1 t1 7.21.7 t1]` |
| 3 | a tagged OCR'd scan fails a strict subset of the untagged scan | **met** | `TestTaggingAnOCRdScanCostsItNoUA1Clause`: cleared `[6.2 t1 7.1 t11]`, added nothing |
| 4 | every tree nib writes records which of D4's sources produced it | **met** | `TestEveryTreeNibWritesRecordsItsSource`, plus `…CameFromOCRAndNotFromAnAST` (approximate) and `…CameFromNibsOwnFieldList` (exact) |

**Required-run gates, enumerated separately because a gate is not a criterion and nothing else walks
them** (measured at v1.129.61):

| tier | result |
|---|---|
| 0 `go build ./...` | PASS |
| 1 `go test ./...` | PASS |
| 2 `./build/jsdomtest.sh` | **349/349** |
| 3 `./build/uirepro.sh` | **RED — 3 failures, all pre-existing and none from this phase.** Proven in a clean `git worktree` at `7a6d0f8` (v1.129.55) before P06.S04: the identical three. Filed as `/pending 474` |
| 4 `./build/pairrepro.sh` | PASS, both transports |
| 4d `./build/pairrepro.sh -n 4` | PASS, 4-party baton relay, both transports |
| 6 `./build/ceremonyrepro.sh` | **27/0** |

The slice gate fired **once** in the phase — P06.S04 touched `internal/p2p` and ran tiers 4 and 6 at
v1.129.56. S01–S03 and S05–S07 touched none of its paths and printed the line saying so.

**What the phase close found that no slice did, both by measuring the criteria rather than by walking
the inventory:**

- **Four agreeing copies of the tagging law.** By S07 there were four doors building a tree, each
  carrying its own `/MarkInfo` + tier + `orphaned()` post-condition. All four agreed — which is
  precisely ADR-009's case, because agreement between four says nothing about the fifth door P07 will
  add. Collapsed into `claimTagging`, with a guard that asserts **routing** rather than agreement and
  names the offending function and file when a door bypasses it. The door also gained
  `supportsAClaim()`: `orphaned()` cannot be asked before the claim exists, because a document making
  no claim cannot be lying.
- **`AuthorForm` embeds ZERO fonts** where Markdown embeds 2 and the OCR layer 1 — so a described
  form fails `7.21.4.1 t1` and `7.21.7 t1`. D7 already decided this (*"authored text embeds its
  fonts"*) and the rule table assigns it to **P04, which closed without covering the form door**.
  P04's guard could not see it: it counts descriptors carrying `FontFile2` and asserts their
  `/CIDSet` is honest, so **a door that embeds nothing contributes no descriptor and is never
  reached** — the guard asks whether what nib embedded is honest, never whether nib embeds at all.
  Filed as `/pending 479`.

**Two slices deserve their own line because the plan was wrong about them and step zero said so
before a line was written.** S05: `7.18.4 t1` was recorded as out of reach, and that was true only of
dictionary KEYS — S07 cleared it with a tree. S06: the OCR layer was not "untagged" but marked
`/Artifact <</Subtype /Watermark>>`, which tells a conforming reader to **skip** it, so the one thing
making a scan readable was declared not-to-be-read and ua1 7.1 t3 was satisfied by disclaiming the
content.

**And one defect hid an entire slice.** `anchored()` counted an element as reaching live content only
when the element's own `/Pg` named a live page — true only of MCID-bearing elements. A grouping
element has no `/Pg` by design, and describing an annotation means a `/Form` element whose `OBJR` kid
names the page, so a correctly tagged form scored `anchored == 0`, `orphaned()` called it a lie, and
`AuthorTaggedForm` silently returned the untagged form. **The slice was inert and looked like a
fallback working as designed.** Found by inspecting the output when `tagged=false` had no error
behind it — the instrument that measures every slice of P05 and P06 was the thing that was wrong.

**Residual doubt:** tier 3 is red for three failures this phase did not cause and cannot see from
here, so the one gate that drives the whole app in a real browser is not currently readable as a
gate (`/pending 474`).
### P07 — The pure-Go conformance checker *(done 2026-09-14, v1.129.71)*
**Goal.** Nib's own PDF/UA checker (D6) and the remediation report, with law 4's three verdicts and
law 5's agreement guard against veraPDF over the corpus.

**Exit criteria.** The checker agrees with veraPDF on every corpus fixture or names the rule it
cannot evaluate; no rule is reported as passing that the checker did not actually run; the report
is reachable from the UI and from the CLI.

**Carried in from P04.S04.** *A document carrying non-embedded fonts is refused for UA export with
the reason named* — P04's second exit criterion, moved here because **nib has no UA export door**
(named search, 2026-09-11: four hits, all in test files). This phase is where one first exists, and
where the refusal can name every rule a document fails rather than only the font one. `nonEmbeddedFonts()`
is built and `pdfaBlockers` is the idiom to mirror.

**(phase-open, 2026-09-12 at v1.129.61 — five facts established before the slices were cut.)**

| measured | result |
|---|---|
| the sketch's *"nib has no UA export door"* | **holds**, and the search names itself: `grep -rniE '\bua\b|pdfua|PDF/UA' --include=*.go internal/ cmd/` minus tests and comments returns only `UDPAddr` locals in `ceremonynet.go` and `mcast.go`. `pdfa.go` is the only convert-or-refuse door that exists |
| what the checker can actually read | **everything it needs is already built.** `inspectTags`, `readStructTree`, `checkStructConsistency`, `parentTreeEntries`, `StructureSource`, `contentstream.Tokenize`, `nonEmbeddedFonts`, `watermarkArtifactSpans` and the OC-config reader between them cover every clause P03–P06 measured |
| the clause population | **18 clauses this repo has direct measurements of** — `5 t1`, `6.2 t1`, `7.1 t3/t8/t9/t10/t11`, `7.2 t25/t33/t34`, `7.10 t1/t2`, `7.18.1 t3`, `7.18.3 t1`, `7.18.4 t1`, `7.21.4.1 t1`, `7.21.4.2 t2`, `7.21.7 t1`. Each was taken from veraPDF against a real nib document during P03–P06, so the checker's expected verdicts are not guesses |
| the corpus | `internal/pdfops/corpus_test.go`, **generated and readable** rather than committed binaries, and today it is essentially ONE document (`taggedFixture`). Law 5's agreement guard needs many; P06 already produces the shapes (tagged Markdown, described form, tagged scan, plain scan, untagged page, stamped page, merged pair) |
| `5 t1`'s gate | **this phase is it.** The identification is refused *"until something can say the document conforms"* — P03.S01's decision. Two places record the gate and they disagree: `ua1oracle_test.go` says *"Refused until P05, when it is true"* and `tagmarkdown_test.go` says *"P07's job"*. **P07 is right** — writing a conformance assertion needs something that checks conformance, which P05 is not. The stale note is corrected in S01 |

**`/plan-review` did NOT fire on this phase**: it is not security-, migration- or egress-heavy — no
wire format, no stored-format version, no network path, no credential. It reads documents and writes
one catalog key.

**Firmed slices:**

#### P07.S01 — a rule is a function with three verdicts *(done 2026-09-12, v1.129.63)*
Scope: the checker's spine and law 4, which is the whole design rather than a detail. A rule is a
named function returning `pass`, `fail(what and where)`, or **`cannot check, and why`** — and the
third never collapses into the first. One rule implemented to prove the shape, chosen for having a
reader already: `7.1 t11` off `inspectTags`. Also corrects the stale `5 t1` gate note. Refs D6, law
4, exit criterion 2.
Acceptance:
- ~~The verdict type makes the collapse law 4 forbids **unrepresentable**, not merely avoided — a
  rule cannot return "pass" without having run, and the guard is a mutation that tries.~~
  **AMENDED by the grill before a line was written, and again confirmed in the build:** no type can
  stop a function writing `return Result{Verdict: Pass}` having checked nothing, so the clause as
  written was unachievable and would have been closed over. What a type CAN stop is **absence**
  reading as conformance — which is how this law actually gets broken, by a rule that returns early,
  a map lookup that misses, a `make([]Result, n)`, or a helper that forgets a field. So: the zero
  `Verdict` is `NotRun`, `NotRun` is not one of law 4's three verdicts but the state of never having
  been asked, and no unresolved verdict counts toward conformance. Probed by making `Pass` the zero
  value, which turns three assertions red at once.
- A rule that cannot evaluate a document says which rule and why, and the report prints it as
  distinct from a pass.
- The registry is enumerated from the code, with a floor, the way `tagFates` is — a rule that is
  never registered is a clause nobody checks and looks identical to one that passes. **Built as a
  `go/ast` scan over every `register(Rule{Clause: …})` literal in the package**, compared both ways
  against what the registry holds, with two stimulus floors: a scan that parsed nothing and a
  registry that is empty each agree with the other vacuously. Probed with a rule written in a
  function nothing calls.

#### P07.S02 — the catalog and metadata rules *(done 2026-09-14, v1.129.64)*
Scope: the clauses readable without a structure tree — `7.1 t8`, `7.1 t9`, `7.1 t10`, `7.2 t33`,
`7.2 t34`, and `7.10 t1`/`7.10 t2` over every optional-content configuration. P03 and `/pending 473`
already write all of these, so each has a document that passes and a document that fails.
Acceptance:
- Each rule agrees with the measurement its own phase recorded, on the document that phase produced.
- `7.10` is checked over **every** configuration dictionary (`/D` and each `/Configs` entry), because
  the clause says *"each"* — the same population `honestOptionalContent` corrects.
- A document with no `/Metadata` at all makes `5 t1` and `7.1 t9` **not applicable**, not passing —
  the distinction that makes law 4's third verdict load-bearing rather than decorative.

**(grill + build, 2026-09-14 — amended in three places, all measured.)** `7.2 t34` is a THREE-way
rule, not two: with no catalog `/Lang`, an untagged document is a provable `Fail`, but a tagged one
is `CannotCheck` until S03 walks the tree, because an element may declare the language. `5 t1` lands
here able to reach only `NotApplicable` and `Fail` — ~~`Pass` becomes reachable at S07, which is the
honest order because nothing writes the identification until then~~ **(struck at P07's close: S07 closed
WITHOUT writing the identification, so no nib document reaches `Pass` on `5 t1`; see S07 and
`/pending 486`)**. **Two branches cannot be driven
end to end**: `open` reads with `ReadValidateAndOptimize`, so the checker only ever sees documents
pdfcpu accepts — pdfcpu REFUSES a metadata stream whose `/Type` is not `/Metadata` and PANICS writing
a non-boolean `/DisplayDocTitle`. Both branches are kept (another producer's document can arrive in
either state) and driven by calling the rule directly; the boundary is itself asserted, so a change
to `open` goes red. **Live verification against veraPDF: 45 of 45 verdicts agree** over five
documents × nine clauses, with one resolution limit S05 must close — veraPDF reports passed checks
only in aggregate, so "not listed" mixes passed with not-applicable.

#### P07.S03 — the structure rules *(done 2026-09-14, v1.129.66)*
Scope: `6.2 t1`, `7.1 t3`, `7.1 t11`, `7.18.4 t1` — the clauses that need the tree and the content
stream. `7.1 t3` is the hard one and the one nib has the most evidence about: it needs the walker to
find content that is neither inside a marked-content sequence nor an artifact, which is exactly what
`watermarkArtifactSpans` and `textOperatorSpans` were built to see.
Acceptance:
- `7.1 t3` reports the OFFENDING content, not just the verdict — P01 spent a slice discovering
  veraPDF points at `xObject[0]/contentStream[0]/content[2]`, and a checker that only says "fails"
  reproduces the problem it exists to solve.
- ~~`7.18.4 t1` follows the `OBJR` linkage both ways, as P06.S07's reader does.~~ **AMENDED by law 5
  at P07.S05, 2026-09-14**: veraPDF reads the clause from the annotation up. Measured one link at a
  time — `OBJR` removed PASSES, `/StructParent` removed FAILS, element retyped `Div` FAILS — so the
  rule no longer requires the back-link, and a document missing it is a PASS row in its test rather
  than a failure the oracle does not report. The writer still writes the `OBJR`; the structure tree
  needs it for a reader walking down.
- Every document P06 produces gets the verdict P06 measured for it, per clause.

**(grill + step zero, 2026-09-14 — three findings, one of them a defect P06 closed over.)**

- **P06's tagged Markdown was never reachable — `/pending 481`.** The acceptance clause above needs a
  tagged Markdown document produced through a product door, so the grep went looking for one:
  `tagMarkdown` had callers ONLY in `_test.go` files, and `ConvertDocToPDF` called
  `mdpdf.ConvertWithFaces` directly. Every Markdown file a user converted was untagged while P06's
  criterion 1 was met by a test. The zero-caller scan could not see it — `tagMarkdown` is
  unexported. Fixed in its own commit before this slice (v1.129.65), with a logged fallback to the
  untagged render when tagging refuses, and the criterion's reader moved to the product door.
- **The checker walks the tree itself rather than borrowing `pdfops`' model.** A checker reading
  documents through the writer's own model agrees with the writer by construction; `pdfops`' own
  tests keep `countElementsIndependently` for that reason. Two rules, each with one door — *how to
  build* in `pdfops`, *what conforms* here — not ADR-009's two opinions about one.
- **`7.2 t34` joins this slice.** P07.S02 shipped it answering `CannotCheck` for any tagged document
  and named S03 in the reason it gives users; leaving that sentence false would be the stale-gate
  shape `/pending 433` is about. With the walk, each piece of text resolves: artifact → no language
  needed, MCID → element or nearest ancestor `/Lang`, neither → `Fail`. A `/Span <</Lang>>` property
  list is deliberately NOT read, because P06.S04 measured that veraPDF does not credit it.
- **P01's real tagged PDFs are gone.** `find / -name boi.pdf -o -name adgm_va.pdf` returns nothing;
  only their names survive in this plan. S05's corpus is regenerated from D9's LibreOffice recipe.

#### P07.S04 — the font rules *(done 2026-09-14, v1.129.67)*
Scope: `7.21.4.1 t1`, `7.21.7 t1`, `7.21.4.2 t2`. `nonEmbeddedFonts()` exists and P04 built the
`/CIDSet` reader; this is mostly wiring, plus the ToUnicode rule which is new. Refs `/pending 479`,
which this slice's own reader will confirm or refute.
Acceptance:
- The three rules reproduce P04's and P06's measurements, including `AuthorForm`'s **zero** embedded
  fonts — so `/pending 479` gets a standing reader rather than a one-off measurement.
- A Base-14 font is reported as not embedded, and the report says which face on which page.

**(grill + step zero, 2026-09-14 — AMENDED. Four measurements, each of which changes a rule.)**

| measured on veraPDF ua1 | result |
|---|---|
| a non-embedded font in resources, never selected by `Tf` | passes 7.21.4.1 — **"used for rendering" is literal**, so the rule reads fonts that text operators SELECT, including in widget appearance streams (veraPDF located the form's Helvetica at `annots[0]/appearance[0]/contentStream[0]/operators[10]/font[0]`). `pdfops.nonEmbeddedFonts` walks the whole xref table and would over-report |
| render modes | `3 Tr` (invisible) is exempt from 7.21.4.1; `7 Tr` (clip) is NOT; `Q` restores the mode and `ET` does not reset it |
| 7.21.7 t1 | Helvetica maps with or without `/Encoding`; Symbol and ZapfDingbats do not; **invisible text is NOT exempt** — the asymmetry with 7.21.4.1 |
| 7.21.4.2 t2's population | pdfcpu keeps 3359 glyph slots and empties all but 8. A `/CIDSet` of the non-empty glyphs FAILS; one of every `maxp` slot PASSES. **The population is `numGlyphs`**, so the rule reads `maxp` alone |

**`/pending 479`'s second clause was misattributed, and is corrected in the entry.** A text-only form
fails 7.21.4.1 t1 alone; a checkbox-only form fails 7.21.7 t1 as well — the ZapfDingbats in the
checkbox's `/AP /N /Yes` appearance. The P06 fixture had both field kinds, so the two clauses arrived
together and were read as cause and effect. Embedding a text face would not clear 7.21.7.

**The `/CIDSet` must be EXACT, and nib had it half right until law 5 said otherwise.** The first
rule checked only that every glyph slot was covered. The live agreement run at this slice's close
disagreed on one document — nib passed a set veraPDF failed — and the measurement that followed:
exactly slots 0…`numGlyphs`−1 PASSES; the same set with the last byte's unused bits on FAILS; an
extra byte of set bits FAILS. Over-claiming is a breach, the rule now refuses it, and the fixture that
produced the disagreement had written `0xFF` into every byte. **That is the first finding law 5's
check produced against the checker itself**, which is the reason the law exists.

**Unmeasured cases are `CannotCheck`, never inferred.** 7.21.7 for a non-Base-14 font without a
`/ToUnicode` (including `Identity-H`) is not something this slice measured, so it is reported as a
gap nib names rather than a verdict nib guesses. S05's corpus is where those rows get measured.
#### P07.S05 — the oracle validates the checker *(done 2026-09-14, v1.129.68)*
Scope: **law 5, and the slice the whole phase rests on.** For every corpus document, nib's verdict
per clause agrees with veraPDF's, or nib says `cannot check`. The corpus grows from one document to
the shapes P06 produces. Refs law 5, D12, exit criterion 1.
Acceptance:
- Agreement is asserted **per clause per document**, both directions: nib may not fail what veraPDF
  passes, and may not pass what veraPDF fails. `cannot check` is permitted against either and is
  counted, so the count cannot quietly grow.
- The corpus has a **floor**: a minimum document count and a minimum count of clauses actually
  exercised, or the guard passes over an empty set — the shape `ua1oracle_test.go` already uses.
- The guard SKIPS loudly when veraPDF is absent and says the criterion is unchecked, never passing.

**(grill + step zero, 2026-09-14 — the gap S02 handed this slice is closed, and the guard's first run
found a subject-definition defect.)**

- **This heading was missing.** P07.S04's plan amendment replaced the text through this line and did
  not put it back, and that went into `ff92db6`. Resume scans by heading, so a session dying there
  would have skipped this slice. Restored before any S05 work.
- **veraPDF can tell "passed" from "no subject" after all.** `--passed` lists every rule with
  `passedChecks` and `failedChecks`; a rule at `0`/`0` had nothing to check. So the guard asserts
  THREE states strictly — veraPDF failed ↔ nib `Fail`, passed with checks ↔ `Pass`, no subject ↔
  `NotApplicable` — rather than scoring nib's `Pass` and `NotApplicable` both as agreement, which
  S02's live check had to.
- **First run: 191 of 195 (document, clause) pairs agree strictly**, over 13 documents, with zero
  pass/fail contradictions and every no-subject rule matching `NotApplicable` exactly. **The four
  mismatches were one defect in nib**: 7.21.4.2 t2 on an embedded CID font with NO `/CIDSet` —
  veraPDF runs the check and passes it, nib called it not applicable. The clause's subject is the
  embedded CID font, not the `/CIDSet`; the rule is corrected.
**The guard's first run, and what it found beyond the draft — 298 of 300 pairs, two more defects in
nib.** 7.18.4 t1 (above, in S03's amended clause) and 7.2 t33, whose subject is language-alternative
metadata text rather than the packet merely existing: measured with no catalog `/Lang`, `dc:title`,
`dc:description` and `dc:rights` as `rdf:Alt` with `x-default` FAIL; `dc:creator` as `rdf:Seq` and
`xmp:CreateDate` alone have NO SUBJECT; and `dc:title` with `xml:lang="en"` PASSES — an alternative's
own language satisfies the clause. P07.S02's rule read the catalog key alone. Both rules corrected,
and each measured shape is now a corpus document so the rules cannot drift back.

**The corpus also needed one fixture to reach every state.** `7.1 t9` was never FAILED by any
product door; a packet without `dc:title` reaches it. `5 t1` is never PASSED ~~until S07 writes the
identification, and is declared not yet reachable with S07 as its gate — checked against the plan's
marker, so S07 cannot ship without a corpus document that reaches it~~ **(re-gated at S07's close: S07
wrote nothing, so the row's gate is `/pending 486`, and a pending-item gate is not checked against a plan
marker)**.

- **A default LibreOffice conversion is tagged** — an ODT came out with 12 anchored elements and
  records no nib source, which is right for a foreign producer — so real third-party structure joins
  the corpus through `ConvertOfficeToPDF`, skipped loudly only where LibreOffice is absent. HTML is
  not an office extension nib accepts, so the fixture is a generated ODT.

#### P07.S06 — the UA export door and the report *(done 2026-09-14, v1.129.69)*
Scope: the carried-in P04.S04 criterion. One door that either exports a UA-labelled document or
refuses with **every** reason named, mirroring `pdfaBlockers`. The report is reachable from the UI
and from the CLI. Refs P04.S04's carried criterion, exit criterion 3.
Acceptance:
- A document with non-embedded fonts is refused with the font named — and with every OTHER failing
  rule named in the same refusal, which is why this phase is where the criterion could finally move.
- The report distinguishes law 4's three verdicts visibly, and a `cannot check` is never rendered in
  the same style as a pass.
- Both surfaces reach the same door; the UI and CLI do not each decide what conformance means.

**(grill, 2026-09-14 — AMENDED in scope, not in intent: this slice builds the door's REFUSAL and its
report; ~~P07.S07 attaches the export to the same door~~ — **struck at P07's close: S07 measured that nib
cannot honestly label a document, so the door refuses and reports and nothing exports a label.**)**

- **No document can be conformant ~~until S07~~** — and after S07's measurement, not at all by nib's
  hand. `5 t1` fails on every document with a metadata packet and has no subject on the rest, while 7.1
  t8 fails those. So an export branch here would be code no document can reach. ~~Writing the
  identification is S07's whole subject.~~ **S07 refused it: a 15-of-106 checker cannot support the claim.** The carried P04.S04
  criterion is itself a refusal — *"refused for UA export with the reason named"* — and the refusal is
  what this slice builds, naming every failed clause and every clause nib could not check.
- **The door lives in `internal/uacheck`, not `internal/pdfops`.** `uacheck`'s in-package tests import
  `pdfops`, so `pdfops` importing `uacheck` is an import cycle. `uacheck.CheckForUA` returns the report
  and the refusals; the server route (`GET /api/uacheck`, read-only like `/api/scan`) and `nib ua` both
  call it, and a repo-root guard refuses a direct `uacheck.Check` from either surface.
- **The verdict travels as a word.** `uacheck.Verdict` is an integer whose zero is `NotRun`; the server's
  own response type carries the string, and it is entered in `published.test.mjs`'s table, because a
  `writeJSON` of `uacheck.Report` would have put its fields outside the scan that guarantees each is read.
- **"Never rendered the same as a pass" is asserted in WORDS as well as style** — WCAG 1.4.1, and P02's
  lesson that colour alone is not a distinction: a cannot-check row says "? Nib could not check", is
  italic and yellow, and never carries a tick.

#### P07.S07 — ~~nib writes the identification, because now something can say it conforms~~ nothing can yet say a document conforms, so nib does not write the identification *(closed 2026-09-14, v1.129.70 — NO IDENTIFICATION WRITTEN; the slice corrects an overclaim instead)*
Scope: `5 t1` — `pdfuaid:part 1` in the XMP packet. **The clause P03.S01 deliberately refused**, and
the last one a tagged Markdown document fails. Written ONLY where the checker reports no failure and
nothing it could not check. Refs law 1, law 4, P03.S01's decision, P06's criterion 1.
Acceptance:
- The identification is written only when every registered rule returned `pass` — a single `fail` or
  a single `cannot check` withholds it, and that is asserted in both directions.
- A tagged Markdown document then fails **no** `ua1` clause at all, which is the first time anything
  nib produces can say that. Measured on the oracle.
- Nothing writes the assertion outside that door, guarded the way `claimTagging` is — by routing,
  not by agreement between sites.

**(grill, 2026-09-14 — OVERTURNED by measurement before a line was written. The acceptance above is
struck in effect: building it would violate ADR-031's law 1, which this plan adopted.)**

The premise was that P07's checker is "something that can say the document conforms". It is not, and
the step zero that tested the premise also found the overclaim S06 had just shipped:

| document — tagged Markdown, titled, `/Lang` | veraPDF: all **106** ua1 rules | nib: **15** rules |
|---|---|---|
| no identification | fails `5 t1` only | not conformant |
| `pdfuaid:part 1` written into the existing packet | **fails nothing** — the first fully conformant document nib has produced | conformant |
| a heading that skips a level, identified | **fails `7.4.2 t1`**, a rule nib does not implement | **conformant** |
| a table, or a link, identified | fails nothing | conformant |

- **"Every registered rule returned pass" is not conformance, measured.** nib implements 15 of the 106
  rules veraPDF evaluates. The heading-skip document passes all 15 and fails the profile, so the door
  this slice specified would have written a conformance assertion over a non-conformant document —
  law 1's third forbidden thing, by name. Implementing 7.4.2 t1 closes that instance and not the class:
  91 rules remain, each a place the same false label can come from.
- **Even the one fully conformant document is not reachable through a product door.** It needs a
  catalog `/Lang`, and the only product caller of `SetLang` is the OCR route; nib is not told a
  Markdown document's language (`/pending 471`). A labelled Markdown export would first need that
  decision.
- **S06 overclaimed in words.** The README said `nib ua` exits 1 "when the document is not PDF/UA",
  which makes exit 0 read as "is PDF/UA"; the door's doc comments said it answers "may this document be
  called PDF/UA". The heading-skip document exits 0. What this slice builds instead is the correction —
  every surface says *"passes every clause nib checks"* and names the coverage — with tests that fail
  if any surface claims conformance.

So nib does not write the identification, and the question of how it ever honestly could is a scope
and priority decision filed for Dan (`/pending 486`): implement the profile, label only nib's own
fully verified output once a language source exists, or never label. `5 t1` stays declared not yet
reachable in the oracle guard, re-gated on that item rather than on this coordinate. The 7.4.2 t1
instance itself — nib's tagged Markdown fails it whenever the source skips a heading level — is filed
as `/pending 487`.

**(phase close, 2026-09-14, v1.129.71.)**

| exit criterion | verdict | evidence |
|---|---|---|
| the checker agrees with veraPDF on every corpus fixture or names the rule it cannot evaluate | **met** | `TestTheOracleValidatesTheChecker` — 22 generated documents plus a LibreOffice ODT, every clause compared both ways; `knownCannotCheck` is empty; `notYetReachable` holds only `5 t1 passed` (`/pending 486`) |
| no rule is reported as passing that the checker did not actually run | **met** | `NotRun` is the zero verdict; `runOne` turns a panic into `cannot check`; `Report.Conformant` requires every result `pass` or `not applicable` |
| the report is reachable from the UI and from the CLI | **met** | `GET /api/uacheck` + `#uaBtn`, `nib ua IN`; `TestTheUIAndTheCLIReachTheSameConformanceDoor` checks both call `uacheck.CheckForUA` |
| carried from P04.S04: non-embedded fonts refused with the reason named | **met** | `TestUARefusesWithEveryReasonAndNamesTheFont` — `7.21.4.1 t1 fails` names Courier, beside the other failing clauses |

**Found at the close — a gated exemption P07 had not discharged.** `zerocaller_test.go` exempted
`StructureSource` as *"gated — PLAN-accessibility.md P07"*: D4 says the user is told which tier
produced the tree, and P07's report was named as where that line belongs. Tier 1 went red on the
marker (`TestNoGatedExemptionOutlivesItsCoordinate`). The caller is now built rather than re-gated:
`pdfops.DescribeStructureSource` is one sentence per tier plus one for an unrecorded tree, shown by
the report modal (`uaReportResponse.Structure`) and printed by `nib ua` on stderr; the root guard
checks both surfaces call it, and the exemption row is removed. Six mutations red (server field, CLI
line, CLI routing, UI render, unrecorded described as exact, generic described as exact).

**Found at the close — `/pending 481`'s fix made Markdown conversion quadratic.** Tier 4d builds a
20,000-clause Markdown fixture with `nib office`; since v1.129.65 routed conversion through
`tagMarkdown`, that step ran over twenty minutes (process observed at 21:52 elapsed, 27 CPU-minutes).
Tiers 0–3 never convert a large document. Profile at 2,000 clauses: untagged render 2.8 s,
`tagMarkdown` 15.0 s — `addMarkedElementUnder` and `addMCIDTo` rebuilt every ParentTree slot list
(`parentTreeEntries`) and walked the page tree from its root (pdfcpu `PageDict`) once per marked run.
Fixed with a key-only lookup (`parentTreeKey`) and a per-tree page cache (`structTree.page`), which
`tagOnePage` and `tagOCRPage` also use: 2,000 clauses 15.0 s → 5.0 s. Guarded by
`TestMarkingARunCostsTheSameHoweverManyRunsCameBefore` — 4× the runs measured 3.9–4.7× green, 15.0×
and 14.0× with the full scan restored in each writer separately. Blind spot declared in the test: one
page, so the page cache is not exercised there. At 20,000 clauses: tagged conversion 142 s against
36.6 s untagged (from over twenty minutes).

**And the tier-4d failure it surfaced was mostly not tagging.** Hop 1's spoken check went up 132 s after
the hop began, past the harness's 60 s watcher, while other runs loaded the machine. Unloaded, the
content anchor on the convened tagged fixture is 27.3 s + 10.3 s; on the untagged render 33.5 s + 7.7 s
for the digest and record parse alone — tagging adds ~12%. The anchor is quadratic in pages on its own
(pdfcpu's per-page `PageDict` walk, and font programs re-decoded on every page): `/pending 488`.

**Required-run gates, v1.129.71:**
- T1 `go test ./...` — green.
- T2 `jsdomtest.sh` — 354/354.
- T3 `uirepro.sh` — RED for the 3 pre-existing failures (`/pending 474`: `blockink.test.mjs`, the CJK
  CMap text, "leaves the shared server"). A fourth, `no console errors`, appeared once while a tier-1 run
  overlapped it and did not recur on an unloaded re-run (138 pass, 3 fail).
- T4 `pairrepro.sh` — PASS over both transports.
- T4d `pairrepro.sh -n 4` — FAILED loaded (above), PASS unloaded on the fixed tree: 4 instances, both
  transports, the interrupt hop's words shown after 38 s.
- T6 `ceremonyrepro.sh` — 27 pass, 0 fail.

### P08 — The autotagger *(done 2026-09-14, v1.129.82)*
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

**(phase-open, 2026-09-14 at v1.129.71 — facts established before the slices were cut.)**

| measured | result |
|---|---|
| a positioned-run reader in Go | **none.** `grep -rlnE '"Tm"\|"Td"\|"TD"\|"T\*"' --include=*.go internal mdpdf` minus tests → 0 files; the only run-shaped types are `opSpan` and `artifactSpan` (byte offsets, no geometry). `internal/contentstream` tokenizes and edits; nothing tracks text state |
| a width reader | **none.** `grep -rnE '"Widths"\|"DW"\|\["W"\]' --include=*.go internal mdpdf` minus tests → 0. The core-font door exists: `mdpdf.CoreWidth(text, fontName string, size int)` — an **integer** size, which a reader holding `9.5` cannot pass without truncating |
| a grouping door | **none for positioned text.** `groupWords` (`tagocr.go`) groups by tesseract's own block/paragraph/line ids — it takes a grouping, it does not infer one |
| `PLAN-text-reflow.md` P02–P04, the dependency | **not started** — no marker on P02 (width reader), P03 (positioned runs), P04 (lines and paragraphs). Its D10 settles the order: *"Whichever plan reaches it first builds it; the other calls it."* |
| reflow's 18-PDF font corpus (its D12) | **not on disk.** No `*corpus*` directory under `~` (depth 6); zero PDFs under `~` or `/tmp` with a 2026-09-06 mtime. The same loss P01's `boi.pdf`/`adgm_va.pdf` suffered. The ~1,660 PDFs under `~` are personal documents and are not a corpus |
| a source of TRUTH for inferred structure | **LibreOffice 24.2.7.2 is installed**, and its default conversion is tagged (P07.S05 measured it). A document LibreOffice tags, stripped of its tree, is an arbitrary PDF whose correct answer is known |
| pdf.js for seam S7 | vendored at `web/vendor/pdfjs/` (`pdf.min.mjs`); `pdfjs-dist` is not in `node_modules` |
| another session in this checkout | one other `claude` process has cwd `/home/dan/repos/nib`; every file modified in the last hour is this session's and its untracked `PLAN-returned-document.md` was last written 2026-09-08 — idle, so no worktree |

**Sequencing, decided (rung 2, reversible):** P08 builds `PLAN-text-reflow.md`'s P02, P03 and P04 as its
first three slices, each against THAT plan's exit criteria as written, and each marker is written in
both plans. Reflow's D10 already assigns the work to whichever plan arrives first; the alternative —
parking the autotagger behind another plan nobody is building — leaves both unbuilt.

**The metric exit criterion 1 needs, stated:** over the truth corpus (S04), each document's inferred
tree is scored against LibreOffice's own tree on two numbers — **paragraph-boundary F1** and
**heading/body agreement** — and "no tree" scores zero recall on both. A proposal is measurably better
when both numbers are above zero on every document and their corpus means clear floors S04 records.

**`/plan-review` does NOT fire**: no wire format between machines, no stored nib format, no network path,
no credential. The commit writes a structure tree into a document the user holds, through the doors
P05–P06 built. **`/deepdive` does not fire at phase open** — the seam it would read (Detect's
client-side proposer vs a server-side one) is S06's step zero, measured there.

**Firmed slices:**

#### P08.S01 — the width reader (`PLAN-text-reflow.md` P02) *(done 2026-09-14, v1.129.73)*
Scope: given a font dictionary and a code, the advance — `/Widths` + `/FirstChar`, all three `/W` range
forms, `/DW`, and the core fonts through `mdpdf.CoreWidth`. Refs reflow D2, D3, D5, law 2.
Acceptance:
- Every lookup returns a width AND its source (`Widths` \| `W` \| `DW` \| `std14` \| `none`); no path
  returns a silent zero — a probe that removes the `none` branch goes red.
- All three `/W` forms parse, each driven by a fixture carrying exactly that form, so a form missing from
  the corpus is a coverage gap and not a pass.
- A fractional size is measured without truncation through the core-font door (measured against a
  `CoreWidth` call at the integer size, scaled).
- A census over the generated corpus is a guard: a regression in coverage is red.

**(build, 2026-09-14 — what the step zero measured, and the pins it produced.)**

| measured | result |
|---|---|
| width forms nib's generated documents carry | converted Markdown: Type0 fonts, `/W` on the descendant, form `c [w …]` only; the core-font page: Courier, no `/Widths`; LibreOffice (`txt`): TrueType `/Widths` with `/FirstChar 0` over a re-encoded 25-code subset. **No generated document carries `cfirst clast w` or `/DW`** — both are driven by hand-built dictionaries |
| a CID width parser to reuse | none: pdfcpu v0.13.0 writes `/W` (`font/fontDict.go:896`) and validates `/DW` and never reads either |
| pdfcpu's core metrics on a code its WinAnsi map lacks | **1000, unmarked** (`internal/corefont/metrics.CoreFontCharWidth`). 40 of 256 codes |

- **Pin — two sources the acceptance did not list.** The specification supplies them and a viewer
  uses them: `MissingWidth` (the descriptor's width outside `/Widths`) and a CID font's default `/DW`
  of 1000 when none is declared — named `DW default`, apart from a declared `DW`, so a census can tell
  an assumed number from a stated one.
- **Pin — the core-font path refuses what pdfcpu invents.** The 1000 substitute is detected by
  measuring the code in Courier, where every real glyph is 600; Symbol and ZapfDingbats have maps no
  such test covers and read `none`. And the metrics are WinAnsi's, so the std14 source applies under
  `/WinAnsiEncoding`, and under StandardEncoding (named, or implied by no `/Encoding`) for printable
  ASCII except 0x27 and 0x60, whose glyphs StandardEncoding names differently. **Revised at P08.S06c:**
  this pin first said a core font with no `/Encoding` reads `none` throughout, and tier 3 found the cost
  — an old PDF's runs measured zero wide, so a proposed element had no width to outline. A `/Differences`
  dictionary still reads `none`; `NameEntry` returns nil for it as for an absent key, which the decoder
  had confused too.
- **Pin — one code per `CoreWidth` call, at size 1000.** `CoreWidth` folds valid UTF-8, and two code
  bytes can spell it; one byte never can. Its size is an int, and at 1000 its user-space width is the
  glyph-space width, which `advanceAt` scales to any real size.
- **Pin — "all three `/W` forms"** is read as `c [w]`, `c [w1 … wn]` and `cfirst clast w`; the
  specification defines two shapes and the first is the one-element case of the second. A `cfirst
  clast` range is clamped to 16 bits.
- **Pin — no production caller yet.** `readFontWidths` is unexported and called only by tests until
  S02; `/pending 481` is what a door with test-only callers costs, so S02's acceptance is where its
  first product caller is asserted.

Census: `std14` 95, `W` 32, `Widths` 17, `none` 0. Eight mutations red, each against its own
assertion: `none` → `Widths`, the unmapped-code guard, the encoding check, the Symbol exclusion, the
CID ceiling, the default-DW source, the descendant check, the truncated-range bound.

#### P08.S02 — positioned runs (`PLAN-text-reflow.md` P03) *(done 2026-09-14, v1.129.74)*
Scope: read a page into runs — text, font, size, position, width — from a text-state machine over
`contentstream.Tokenize`: `BT/ET`, `Tf`, `Tm`, `Td`, `TD`, `T*`, `TL`, `Tc`, `Tw`, `Tz`, `Ts`, `cm`,
`q/Q`, `Tj`, `TJ`, `'`, `"`, and `Do` into form XObjects; text through `/ToUnicode` where present.
Refs reflow D4, D6, seam S7.
Acceptance:
- Run text and run count agree with pdf.js `getTextContent` on the corpus (seam S7), asserted at a tier
  that loads the vendored `pdf.min.mjs`.
- An image-only page returns zero runs and says so structurally, not as an empty slice.
- A malformed document is contained per reflow D6, and the containment is probed non-zero.

**(build, 2026-09-14 — step zero, and the pins it produced.)**

| measured | result |
|---|---|
| a `/ToUnicode` reader | **none.** `grep -rnE "bfchar\|bfrange"` over non-test Go → 0; pdfcpu's only CMap code WRITES one (`font/fontDict.go:584-607`) |
| a byte → character table | none exported by pdfcpu (`DecodeUTF8ToByte` is the reverse); `golang.org/x/text` v0.40.0 is already a dependency (`xfdf.go`), so `charmap.Windows1252`/`Macintosh` cost no new module |
| what the two producers emit | mdpdf: `Tf` in one `BT…ET`, drawing in the next with no `Tf`; position by `cm`; two-byte Identity-H codes in literal strings; `bfchar`-only CMaps. LibreOffice: `Td` + hex-string `Tj`, one-byte codes, TrueType with no `/Encoding`, `bfchar`-only CMap |
| pdf.js in this environment | the vendored 6.0.227 build does not load in Node 20: `DOMMatrix`, then `Promise.withResolvers`, `Promise.try`, `Uint8Array.prototype.toHex` are missing, measured in that order. With those filled (no rendering path touched) `getTextContent` runs |
| pdf.js's unit of text | **not a show operator.** One LibreOffice `Tj` became three items split at a space, and every line ends with an empty end-of-line item |

- **Pin — "run text and count agree with pdf.js" is asserted as TEXT and BASELINES.** The two readers
  cut text differently by construction, so an item count cannot agree. What both must produce however
  they cut is the page's text (whitespace removed) and the set of baselines it sits on (half-point
  resolution); a dropped run, a mis-decoded code or a misplaced line fails one or the other. Agreed on
  all three corpus documents.
- **Pin — the pdf.js side is `test/pdfjs/textcontent.mjs`**, loading the vendored build the app ships,
  run from a tier-1 Go test when `node` is on the path. Absent, the test SKIPS and says why.
- **Pin — the string decoder lives in `pdfops`, not `contentstream`.** That package's own comment says
  tokens are spans and *"a caller that does need [a value] decodes the span itself"*.
- **Pin — text is decoded only from what the document says.** `/ToUnicode`; else WinAnsi, MacRoman, or
  StandardEncoding's printable range where the font names it. `/Differences`, a Type0 font under a
  CMap other than Identity, and a symbolic font with no map are not guessed: the run advances and says
  `decoded == false`. A one-byte `ToUnicode` on such a font does not make it readable.
- **Pin — `noText` is "draws no glyph".** The first cut counted show operators; a probe found it
  indistinguishable from counting runs except for an empty `()` show, which no caller distinguishes, so
  the counter was removed.
- **Pin — containment fires, and the corpus never reaches it.** A panic under the reader is driven
  directly and returned as that page's error; every truncation of a real Markdown page (all prefixes)
  reads without reaching it.
- **Pin — still no production caller.** `readPageRuns` is called by tests until S03's grouping; S03's
  acceptance carries the first product path, for the reason S01's pin gave.

Eighteen mutations red, each against its own assertion: the font reset at `BT`, `Q` as a no-op, `cm`,
`TD`'s leading, `Tc`, `Tw`, `Tz`, `Ts`, the `TJ` sign, the form `/Matrix`, the depth ceiling, one-byte
CIDs, the unparsed-CMap guard, `noText` inverted, containment removed, the `bfrange` increment, the
StandardEncoding quotes, WinAnsi decoding. Three survived the first round and each got a test.

#### P08.S03 — lines and paragraphs (`PLAN-text-reflow.md` P04) *(done 2026-09-14, v1.129.75)*
Scope: runs → lines → paragraphs, once. Refs reflow D10, ADR-009.
Acceptance:
- Paragraph counts match hand-checked expectations on corpus fixtures.
- The grouping rule exists in one place, and a guard asserts nothing else groups positioned runs
  (`groupWords` named as the exemption it is — tesseract's grouping, taken rather than inferred).
- A multi-column page is either handled or reported structurally as outside the tool's competence.

**(build, 2026-09-14 — step zero, and the pins it produced.)**

| measured | result |
|---|---|
| runs on one baseline | abut exactly: nib's Markdown draws a styled line as several runs and each begins where the last ends (176.17 → 176.17), so S01's widths are accurate enough to join by adjacency |
| line and paragraph steps | mdpdf: line 14.85, paragraph 20.85, list item 16.85 (leading = size × 1.35, `paraGap` 6, `tightGap` 2). LibreOffice text: line 11.35, blank-line paragraph 22.70. **LibreOffice ODT default paragraph: 13.8 — identical to its line step** |
| two columns (LibreOffice section, 0.5in gap) | both columns' lines on **identical baselines**, 36pt apart; a trailing `" "` run 3pt wide ends some lines |
| a ragged-right line mid-paragraph | ends 24.6pt short of the column edge, because the next word did not fit |

- **Pin — four signals, any one breaks a paragraph:** a size change; a vertical step over 1.25× the
  column's median; an indent; and a short line **whose shortfall exceeds the next line's first word**.
  Vertical spacing alone cannot be the rule — LibreOffice's default paragraph has none — and "short
  line" alone breaks every ragged line.
- **Pin — a baseline splits at a gap over 1.5 em.** The Markdown bullet sits 1.3 em from its text and
  joins; the column gap is 3 em and splits. Segments then cluster into columns by overlapping extent,
  read left to right.
- **Pin — a layout the rule cannot separate is REPORTED.** A line spanning both columns fuses them
  into one cluster, which then holds side-by-side lines on one baseline; that sets `unsupported` with
  the reason rather than returning interleaved text. Exit criterion 3's "handled or explicitly
  reported": separable columns are handled, the rest reported.
- **Pin — the one door is `readPageLayout`, and the guard is by routing.** Nothing in `pdfops` outside
  `textrun.go` and `grouping.go` may name a run type or call the run reader; a probe file doing so goes
  red naming itself. `groupWords` is the stated exemption — it takes tesseract's grouping and never
  touches a run.
- **Pin — the first word is estimated by character share.** Glyph widths vary; the estimate is only
  compared against a shortfall with a half-em margin, and the hand-checked corpus agrees exactly.
- **Pin — the ODT container has one builder.** `odtDocument` in `lang_test.go` holds LibreOffice's two
  measured refusals (a streamed `mimetype`, a zero `Modified`); `odtWithLang` and this slice's
  two-column fixture both call it.
- **Pin — still no production caller.** `readPageLayout` is S05's input and S06's route is its first
  product path; **S06's acceptance now includes asserting that route calls it**, so the door cannot
  ship test-only the way `/pending 481`'s did.

Hand-checked, text compared exactly: converted Markdown 4 paragraphs; core-font pages 1 and 1;
LibreOffice text 2; LibreOffice two columns 5 in reading order (heading, then the left column's two,
then the right's), `columns == 2`, supported. Eleven mutations red: the join gap, each of the four
signals, the short-line signal without its first word, the unsupported report, column clustering,
whitespace runs, the joining space, and a second-opinion file for the guard.

#### P08.S04 — the truth corpus and the metric *(done 2026-09-14, v1.129.76)*
Scope: LibreOffice-generated documents with known structure — heading levels, paragraphs that wrap,
bulleted and numbered lists, a two-column page — each paired with its tree-stripped copy. Refs D12,
exit criterion 1.
Acceptance:
- LibreOffice's tree is read as the truth (element types and reading order) and the stripped copy has
  no tree — both asserted, or the metric compares nothing.
- The two numbers above are computed per document; "no tree" scores zero recall, asserted.
- The corpus is generated in tier 1 when LibreOffice is present and says it is narrower when absent,
  as P07.S05's oracle does.

**(build, 2026-09-14 — step zero, what the metric measured, and the pins.)**

| measured | result |
|---|---|
| LibreOffice's tree for headings, paragraphs and a list | `Document` → `H1`, `Standard`, `H2`, `Standard`, `L` → `LI` → `Lbl` + `LBody` → `Standard`, `Standard`; RoleMap `Standard → P`; every leaf one MCID, written inline as `/Tag<</MCID n>>BDC` |
| what matches an element to its text | **nothing, before this slice**: S02's runs carried no marked content. A second content walker for the truth would be a second answer to what a page says, so the run reader now carries the MCID in force |
| S03's grouping scored against the truth | headings-and-list **7/7 blocks, boundary F1 1.00**; two columns **5/5, F1 1.00**; a three-page report **14 of 31 blocks, P 0.92 R 0.40 F1 0.56**. Headings 0 everywhere — unlabelled paragraphs, as expected |
| why the report scores 0.40 | **there is no geometric signal to find.** LibreOffice's default paragraph adds no space (13.8 = the line step) and each paragraph's last line ends 28.6pt short of a 542.9pt column where the next word, "Paragraph", is ~57pt — so the short-line signal correctly does not fire. Headings break correctly. The one false boundary is a paragraph continuing across a page |

- **Pin — runs carry the MCID in force.** `BDC` reads `/MCID` inline or through `/Properties`; an
  `/Artifact` sequence masks whatever encloses it; a page's `BDC` around a `Do` tags what the form
  draws; each stream's sequences end with the stream.
- **Pin — two defects the truncation test found in S02's reader while this was built.**
  `matchingClose` returned `len-1` for an unclosed opener, a reversed slice when the opener was the last
  token — the array branch had carried it since S02, unreached because nib's Markdown draws no `TJ`
  array. And the per-stream truncation resliced PAST the stack's length, resurrecting an entry an
  `EMC` had popped — found only because a probe of the `EMC` floor survived. Both fixed, both driven.
- **Pin — the strip is the test's, not `dropTaggingClaim`.** That function's comment keeps it to one
  caller so the decision to strip is taken in one place; building a corpus is not that decision.
- **Pin — "heading/body agreement" is scored as heading F1.** Body is everything else; an agreement
  count rewards a proposer that calls everything body.
- **Pin — the truth's shape is asserted from the source** (7 blocks / 2 headings, 5 / 1, 31 / 7), so a
  truth reader that lost the list-item rule or a heading level fails rather than moving the target.
- **Pin — S05's floors, from these measurements:** boundary F1 no lower than S03's per document
  (1.00, 1.00, 0.56), and heading F1 above zero on every document.

Metric probes red: the document start counted as a boundary, precision replaced by recall, the
list-item rule, the strip keeping the tree, `H2` not a heading, the texts-differ refusal, grouped
paragraphs counted as headings, recall pinned at one. MCID probes red: the artifact mask, the stream
truncation (once a form left a sequence open), the `EMC` floor (once truncation stopped resurrecting),
the inline and `/Properties` lookups, the run's own field, and the old `matchingClose` fallback.

#### P08.S05 — the proposer *(done 2026-09-14, v1.129.77)*
Scope: paragraphs → a proposal: element kind (`H1`–`H6`, `P`, `L`/`LI`), reading order, and the runs
each element covers, from font size relative to the page's body size, weight, and list-marker
prefixes. Writes nothing (law 3). Refs D5, exit criterion 1.
Acceptance:
- Over the S04 corpus both metrics are above zero on every document and clear floors recorded at the
  slice, as a tier-1 guard. S04 measured the floors: boundary F1 at least S03's own per document (1.00,
  1.00, 0.56) and heading F1 above zero on every document.
- The proposer has no path to a writer — asserted by routing, not by reading its output.

**(build, 2026-09-14 — step zero, the scores, and the pins.)**

| measured | result |
|---|---|
| what separates a heading from body in LibreOffice's output | size — 18pt and 16pt against 12pt — and a bold face name |
| how a list label is drawn | a bullet is **U+F095 in OpenSymbol**, a private-use code point and not `•`, as its own run; a number is `1.` in the body font, its own run, **flush against the item text** — so the line's first word is `1.Open` |
| two numbered items alone in a column | ONE paragraph to S03: the column's right edge is their own width, so the short-line signal cannot fire |

| document | boundaries (proposal / grouping floor) | headings | roles |
|---|---|---|---|
| headings, paragraphs and a list | F1 1.00 / 1.00 | F1 1.00 | `H1 P H2 P LI LI P` |
| two columns | F1 1.00 / 1.00 | F1 1.00 | `H1 P P P P` |
| a multi-page report | F1 0.56 / 0.56 | F1 1.00 | 7 headings, no list |
| a numbered list (added to S04's corpus here) | **F1 1.00 / 0.89** | F1 1.00 | `H1 P LI LI LI P` |

Exit criterion 1, *"proposed trees are measurably better than no tree on a stated metric"*: no tree
scores zero recall on both numbers; the proposal scores heading F1 1.00 on every document and never
finds boundaries worse than the grouping it starts from.

- **Pin — the score floors did not catch a wrong proposal, and the first cut shipped past them.** It
  proposed `H1 P P P P` for three numbered items — boundaries and headings both at their floors, every
  item a paragraph — because `1.Open` matched no label. The fix reads the label from the first RUN;
  the guard now also asserts each document's role sequence, counted from its source. **A metric that
  scores boundaries and headings cannot see a list item called a paragraph.**
- **Pin — body size is by characters, not runs.** A page of short headings and one long paragraph has
  more heading runs. Probed: counting runs survived the first test, and a fixture shaped exactly that
  way was added.
- **Pin — no bold-only heading rule.** Nothing in the corpus needs one, and a bold lead-in sentence at
  body size is the first thing it would misread.
- **Pin — a heading is at most three lines.** A long passage set large is large text.
- **Pin — `A.` is not a label.** Uppercase initials begin sentences more often than they number lists;
  lowercase letters, digits and lowercase roman numerals are labels.
- **Pin — the report's body paragraphs remain merged**, per S04's pin: no geometric signal separates
  them, and the proposer adds none. Headings there are found exactly.
- **Pin — still no production caller.** `proposeStructure` is S06's to call.

Eleven mutations red: the heading scale, the line limit, the level order, body by runs, the first-run
label, uppercase labels, private-use bullets, the label split, a list surviving a heading, a list
surviving a paragraph, and a writer call planted in `proposer.go` for the guard. Two survived the first
round (body by runs, a list surviving a heading) and each got a fixture.

#### P08.S06 — review and commit (D5, D10, D11) *(done 2026-09-14, v1.129.80 — in three parts, S06a–S06c)*
Scope: a Tags card in the Document tab. Propose (read-only route) → the proposal drawn over the page →
the user retypes, ignores, or reorders an element → commit writes through the tree model and
`claimTagging` with `sourceInferred`. Refs D5, D10, D11, exit criteria 2 and 3.
Acceptance:
- Nothing is written without a commit — a propose followed by no commit leaves the document
  byte-identical, asserted at the route.
- Every proposed element can be retyped, ignored and reordered, keyboard-only.
- The commit refuses a signed document at the server door.
- The committed tree records `Inferred`, and the report's provenance line says so.
- The propose route reaches the page through `readPageLayout` — asserted by routing — which is the
  first production caller of S01–S03's readers (S03's pin).

**(step zero, 2026-09-14 — SPLIT into three, before a line was written.)** What the slice needs, by
named search:

| measured | result |
|---|---|
| where D10's "Document tab" is | ADR-016's **Document** mode, id `edit` — today labelled **"Page Functions"** in `index.html`, holding page-level groups (Split, Add & Extract, Size & Number, Rotate, Combine & Compare). The Tags card goes in `data-tab="edit"` |
| what Detect's model IS | its guesses become ordinary editable overlays (`makeField`), removed by `clearDetected`, and nothing is written until Save — propose, show, edit, commit |
| a door that brackets show operators for an arbitrary PDF | **none.** `textOperatorSpans` brackets top-level `q … Q` groups, which is mdpdf's shape; arbitrary content need not be grouped, so a commit brackets each run's own show operator and the run reader must carry that span |
| marked content already on a page | detectable through the run reader: runs carry the MCID they were drawn under. A stripped copy keeps LibreOffice's `BDC`s — nesting new MCIDs inside them gives content two owners |
| a server door refusing a signed document | **none** — `grep` over `internal/server` for `sig.State` finds no handler refusing; the one `sign.Verify(...).State` check files an arrival. The commit's refusal is the first |

- **P08.S06a — the commit writer** *(done 2026-09-14, v1.129.78)*. `pdfops` writes a reviewed proposal: each element's runs bracketed
  at their show operators with an MCID, `H1`–`H6` / `P` / `L`›`LI`›`Lbl`+`LBody` through the typed tree
  writers, `claimTagging` with `sourceInferred`. Acceptance: the committed document read back through
  S04's truth reader returns the committed blocks exactly (round trip); `StructureSource` reads
  `Inferred`; veraPDF adds no ua1 failure the untagged input did not have, beyond any a tagged
  document necessarily carries (measured and written down, not assumed); it REFUSES, each driven, a
  document that already has a tree, a page whose runs already sit under MCIDs, and an element whose
  runs are drawn inside a form XObject (bracketing the page stream would describe the `Do`).
- **P08.S06b — the routes** *(done 2026-09-14, v1.129.80, shipped with S06c)*. `GET` propose (read-only, through `proposeStructure` → `readPageLayout`,
  byte-identical document asserted, routing asserted) and `POST` commit (the reviewed elements;
  refuses a signed document at the server door; installs through the same path as every other edit,
  so undo holds). Acceptance: the four bullets above that concern routes.
- **P08.S06c — the Tags card** *(done 2026-09-14, v1.129.80)*. In the Document mode (`edit`): the proposal drawn over the page, each
  element retypable, ignorable and reorderable keyboard-only, and a commit. Acceptance: the keyboard
  bullet above, at tier 2 and tier 3, and the report's provenance line reading `Inferred` after a commit.

**(P08.S06a build, 2026-09-14.)** `commitProposal` in `internal/pdfops/tagcommit.go`.

| measured | result |
|---|---|
| round trip | an untagged Markdown render proposed as `H1 P H2 LI LI P` and committed reads back through S04's truth reader block for block, heading flags included; the tree holds H1 1, H2 1, P 2, L 1, LI 2, Lbl 2, LBody 2; `StructureSource` reads `Inferred` |
| veraPDF ua1, untagged → committed | **6 clauses → 3. Cleared `6.2 t1`, `7.1 t11`, `7.1 t3`; added none.** With a paragraph ignored by the reviewer: still no clause added, and `7.1 t3` still cleared — its text became an artifact |
| a watermarked page | `StampWatermark` draws its text in a form XObject, the proposer proposes it, and the commit REFUSES the whole page |

- **Pin — the run reader now carries each run's show-operator span and whether it was drawn inside a
  form**, because `textOperatorSpans` brackets `q … Q` groups — mdpdf's shape — and arbitrary content
  need not be grouped. A span covers the operator and exactly its own operands (`"` has three).
- **Pin — four refusals, each driven by a real document:** a document with a tree
  (`ConvertDocToPDF`'s tagged output); a page whose runs sit under MCIDs with no tree (the same,
  stripped); a run inside a form (a watermark); a proposal that no longer matches (a different
  document, and — separately — the same document with its last character changed, where every span
  survives and only the text differs).
- **Pin — anything no element covers is an `/Artifact`.** A reviewer's ignored element and any run the
  grouping dropped; left bare it is 7.1 t3.
- **Pin — a list label is its own `Lbl` only where the document drew it as its own run.** Otherwise
  the item is `LI › LBody`.
- **Pin — S03's one-door guard fired on this slice, and it was right to.** `tagcommit.go` names
  `textRun` and `readPageRuns`, and the guard forbade any file but the reader and the grouping from
  touching a run. The commit groups nothing — it re-reads a page to match a proposal's runs by span and
  text — so it is an EXEMPTION named at the site with its reason (ADR-009), not a third owner:
  `groupRuns`, `lineSegments` and `pageRuns` stay forbidden to it, and a second grouping written there
  still fails.
- **Finding, FIXED at v1.129.79 — a watermarked document could not be committed at all.** The
  proposer read the stamp's text like any other and the commit refused what was drawn in a form. The
  stamp was already inside an `/Artifact` sequence on the page; the run reader did not say so. Runs
  now carry `artifact` (any open `/Artifact` sequence, by `BMC` or `BDC`, including an MCID nested
  inside one), and the grouping skips them — one door, so reflow gets the same answer: an artifact is
  not a paragraph. A watermarked page now proposes only its body and commits. The in-form refusal
  keeps a real fixture: the same stamp with its `/Artifact` marker replaced by `/Span BMC`, whose
  form-drawn text is then content. Both mutations red — the flag never set, and the grouping keeping
  artifact runs.

**(P08.S06b + S06c build, 2026-09-14, v1.129.80 — one commit, because `published.test.mjs` fails a
wire shape that has no reader in the client, and the propose response's reader IS the card.)**

`pdfops.ProposeTags` / `CommitTags` (`tagreview.go`) are the exported doors; `GET /api/tags/propose`
and `POST /api/tags/commit` (`internal/server/tags.go`) the routes; a "Tag Structure" group in the
Document mode and `#tagsModal` the card.

| measured | result |
|---|---|
| a review applied to the committed tree | reorder, retype (P → H3) and ignore each read back through S04's truth reader in the reviewed order; lists recomputed from the reviewed order |
| a review that no longer describes the document | the count, an element's text, an unknown or doubled id, a role nobody can choose, everything ignored — `ErrTagsStale` → 409, `ErrTagsReview` → 400 |
| the routes | propose leaves the open document byte-identical; commit installs through `commitMutation` and `POST /api/undo` takes it back; the report's provenance reads "inferred" after a commit and not after the undo; a signed document (`threeSigned`) is refused 409 and left unchanged |
| a 409 in the client | `apiFetch` returns the response and ALSO reconciles tabs against `/api/docs` — the tier-2 case stubs a server that still holds the document, and the review stays open with the server's sentence |
| the outline, in a real browser | **zero width on the first run**: the tier-3 fixture's Helvetica has no `/Encoding`, and S01's width reader refused StandardEncoding, so every run measured `none` and every element's rect collapsed. See the width pin revised under S01 |
| tier 3's cleanup | this file closed its document and then saw a 3-page document it never opened: the server still held one an earlier file leaked (`/pending 474`) and the app activated it. Page divs cannot tell that from a leak of this file's own, so the cleanup counts `/api/docs` |

- **Pin — two defects in S01/S02's readers, found only by drawing an outline.** The width reader refused
  StandardEncoding outright (so lines had no width to group by, and outlines none to draw), and the
  text decoder read a `/Differences` dictionary as an absent `/Encoding` — `NameEntry` returns nil for
  both. Both fixed by presence-and-kind; four mutations red.
- **Pin — the signed-document refusal is the first of its kind at the server.** `refuseSignatureErasure`
  refuses an edit that ERASES a signature; tagging leaves it present and broken, which that door allows,
  so the commit route refuses a signature blob outright.
- **Pin — four standing guards fired and each was right**: the published-observable scan (the returned
  `pdfops.TagProposal` needed a named reader; its nested types are not discovered), law 2's census
  (`CommitTags` declared `untouched` — it refuses a tagged input), `published.test.mjs`, and the
  `resolveDoc` count (26 → 28).
- **Pin — the outline is an estimate**: 0.85 em above and 0.25 em below each baseline; page rotation and
  CropBox are not applied.

Mutations red — tier 2: the commit echoing no text, a move that does not reorder, an ignore dropped, a
refusal that closes the review, focus that does not move the viewer. Server: the signed refusal, a stale
review answered 400, the commit never installed. `pdfops`: the StandardEncoding quote exclusion,
StandardEncoding refused, `/Differences` taken for absent (widths and decoder). Gates: tier 1 green; tier
2 359/359; tier 3 140 pass, 3 fail — the three `/pending 474` failures; the slice gate does not fire
(`internal/server`'s tag routes, not its session, ceremony, delivery or discovery paths).

Nine mutations red: each of the three structural refusals, the text half of the stale check, the
artifact bracketing, the label split, the `Inferred` tier, the element roles, and the guard against
marking one run twice. Two survived the first round — the text check (a different document fails on
its spans first) and the double-mark guard (no case listed an element twice) — and each got a case.
Three span mutations red on the reader.

#### P08.S07 — decide `TagAuthored` (`/pending 480`) *(done 2026-09-14, v1.129.81 — DELETED)*
Scope: 480's own terms — *"if P08 opens and does not call it, that is B."* Decided with S05's fallback in
hand: a page the proposer cannot read has zero runs, and a zero-run page is a scan, whose path is OCR
(`TagOCRLayer`), not a `/Div`. Refs ADR-031's asymmetry, /pending 480.
Acceptance:
- Either a production caller exists and its output is measured no worse than the untagged document, or
  `TagAuthored` and its `sourceGeneric` writer are deleted and the exemption row with them.

**(build, 2026-09-14, v1.129.81 — B, by `/pending 480`'s own rule.)** P08 proposes from the page and
commits through S06a's writer; a page with no text is OCR's. Nothing in P08 calls the generic `/Div`
emitter, and 480 said *"if P08 opens and does not call it, that is B."*

| measured | result |
|---|---|
| production callers of `TagAuthored` | none — named search over Go, docs and scripts; the only references outside the plan's history were tests |
| test uses it served | a "tagged page" fixture in four `uacheck` rule tests and the oracle corpus; a page owning a ParentTree key in `tagform_test.go`; its own emitter tests in `tagemit_test.go`; a census row; the `Generic` tier's rows in `tagsource_test.go` and `claimtagging_test.go` |
| the replacement fixture | `committedProposal` in `uacheck` — propose, accept, commit: a PRODUCT door. The oracle guard re-measured nib's checker against veraPDF on the committed-proposal document and its two mutations, and agreed |

- **Deleted:** `TagAuthored`, `tagAuthoredContent`, `wrapOnePage`, `authoredStructType`, the `Generic`
  tier and its report sentence, the census row, the `zerocaller_test.go` exemption, and the emitter-only
  tests (the `/Div`-not-role decision, the already-marked no-op, the veraPDF clause test — whose
  "6.2 t1, 7.1 t3 and 7.1 t11 clear TOGETHER" check moved into the commit writer's ua1 differential).
- **Kept, because other doors use them:** `ensureStructTree`, `setPageContent`, `alreadyMarked` (and its
  token test), the orphaned floor.
- **Re-pointed at the commit writer, because they are invariants every tree writer owes:** removing
  exactly what was inserted gives back the page's own bytes; the MCIDs in the stream are the ones the
  tree claims; every element's `/P` names the object whose `/K` holds it (the writer nests, so not always
  the root); both halves of the claim are written together. Four mutations red, one per invariant — and
  the first run of the byte test failed on a test bug (the pattern stripping inserted openers did not
  match `/H1`), which the widened pattern fixed.
- **Pin — the tier set is D4's three again.** No production door ever wrote `Generic`, so no document in
  the field carries it; a stray value reads as unrecorded.

**(phase close, 2026-09-14, v1.129.82.)** Every clause split on `and`.

| exit criterion | verdict | evidence |
|---|---|---|
| over the corpus, proposed trees are measurably better than no tree | **met** | `TestTheTruthCorpusScoresTheMetricAtItsBounds` — no tree scores zero recall on both numbers; `TestTheProposalClearsItsFloorsOnTheTruthCorpus` — four LibreOffice documents, heading F1 1.00 on each, boundary F1 1.00 / 1.00 / 0.56 / 1.00, each at or above the grouping it starts from, role sequences asserted. Both ran (not skipped) at the close |
| … on a stated metric | **met** | paragraph-boundary F1 and heading F1, stated at phase open and computed by `scoreBlocks` (`TestTheMetricScoresAWorkedExample`) |
| every proposal is reviewable | **met** | `GET /api/tags/propose` → the Tags card draws each element's outline on its page (`test/ui/tagsreview.test.mjs`) |
| … and editable before it is written | **met** | retype, ignore and reorder, keyboard-only, at tier 2 and tier 3; `TestAReviewIsAppliedInItsOwnOrderWithItsOwnRoles` reads the edits back out of the committed tree |
| nothing is written silently | **met** | propose leaves the open document byte-identical (`TestTheProposeRouteReadsAndChangesNothing`); the proposer has no path to a writer (`TestTheProposerHasNoPathToAWriter`); the commit is an explicit POST that installs as an undoable edit, records `Inferred`, and the report says so. S07 deleted the only other writer of structure over arbitrary pages (`TagAuthored`) |
| carried from S03: a multi-column page handled or reported | **met** | separable columns handled (5/5 blocks in reading order), a fused layout `unsupported` with its reason (`TestALayoutTheRuleCannotReadIsReportedNotGuessed`) |

**Found at the close — text still describing the deleted emitter, and a README with no Tags card.**
`README.md` said Nib "does not yet author" tags and listed three provenance sources, not four; it now
has a *Tag an untagged document's structure* section and names the inferred tier. `PLAN-text-reflow.md`'s
status line still said P02 was next. `ua1oracle_test.go`'s `AuthorForm` reason said "needs the emitter".

**Measured at the close — the review doors' cost, because P07's close found a quadratic writer only tier
4d could see.** A converted Markdown document of repeated heading / wrapped paragraph / two-item list
sections, all elements accepted, timed in-process, unloaded:

| elements | propose | commit |
|---|---|---|
| 200 | 6 ms | 43 ms |
| 800 | 31 ms | 204 ms |
| 3,200 | 109 ms | 754 ms |
| 12,800 | 461 ms | 3.3 s |

Each 4× step costs 4.2–5.0× (propose) and 4.4–4.7× (commit): linear over 200–12,800 elements. Not
measured beyond that, and not on a LibreOffice document or one with many fonts per page.

**Phase review.** Cross-slice read of the server routes and the client: the commit is pinned to the
document the review was opened on (ADR-001, `tagsOwner`), grows the document only through
`commitMutation`'s `byteCapLocked` (ADR-008), passes the ceremony freeze, and refuses a signed document
before `CommitTags` runs; the review body is bounded at 8 MiB. No defect found beyond the stale text above.

**Required-run gates, v1.129.82:**
- T1 `go test ./...` — green.
- T2 `jsdomtest.sh` — 359/359.
- T3 `uirepro.sh` — RED for the 3 pre-existing failures only (`/pending 474`: `blockink.test.mjs`, the CJK
  CMap text, "leaves the shared server"); 140 pass, 3 fail of 143.
- T4 `pairrepro.sh` — PASS over both transports.
- T4d `pairrepro.sh -n 4` — PASS: 4 instances, a 4-party baton relay over both transports (11 s of hops);
  the interrupt leg's verifiers showed their words after 50 s.
- T6 `ceremonyrepro.sh` — 27 pass, 0 fail.

### P09 — The structure editor
**Goal.** The Tags panel and Reading Order view in the Document tab (D10) — inspect, reorder,
retype, set alt text, mark artifacts, and author table header scope.

**Exit criteria.** A tree can be corrected end to end in the UI without leaving nib; every edit is
undoable through the existing history; the panel itself meets P02's keyboard bar.

**(phase-open, 2026-09-14 at v1.129.84 — facts established before the slices were cut.)**

| measured | result |
|---|---|
| a door that hands an EXISTING tree to the client | **none.** Exported structure doors: `StructureSource`, `DescribeStructureSource`, `ClaimsTagging`, `ProposeTags`, `CommitTags`, `AuthorTaggedForm`, `TagOCRLayer`; routes: `/api/uacheck`, `/api/tags/propose`, `/api/tags/commit`. The read model (`readStructTree`) is unexported and carries `/S`, `/Pg`, kids (element, MCID, MCR, OBJR), parent, and `/A` uninspected — no `/Alt`, no `/ActualText` |
| a writer that CHANGES an existing element | **none.** `structwrite.go` builds ADD only (its own header says so); every production writer builds a new tree. `addMarkedElement` had no production caller after P08.S07 and was deleted at v1.129.83 |
| `Figure`, `Table`, `TH`, `/Scope`, `/Alt` anywhere in nib's writers | **none** — mdpdf's roles are body, heading, list item, code, quote, marker |
| a truth source for tables and figures | **LibreOffice HTML import**: `Table › TR › TH/TD`, each `TH` with `/A [<layout> <</O /Table /Scope /Column>>]`; `Figure` with `/Alt` (UTF-16). No object streams, so a same-length byte strip is a valid fixture. Its HTML headings are role-mapped `Heading 1 → P` — a real tree that is wrong, which is what an editor exists for |
| what veraPDF says a missing `/Alt` and a missing `/Scope` cost | stripped `/Alt` adds **7.3 t1**; stripped `/Scope` adds **7.5 t1**; the original fails `5 t1`, `7.1 t9`, `7.1 t10` (metadata/title — not structure) |
| what nib's checker sees of those | **nothing**: it registers 15 clauses (`5 t1`, `6.2 t1`, `7.1 t3/t8/t9/t10/t11`, `7.2 t33/t34`, `7.10 t1/t2`, `7.18.4 t1`, `7.21.4.1 t1`, `7.21.4.2 t2`, `7.21.7 t1`); neither 7.3 nor 7.5 |
| where D10's panel goes | a sidebar content panel (ADR-018/020): a `.tab[data-panel]` header plus a `div.panel`, listed in `SIDEBAR_FOR.edit`, which today is `['commands']` — the first entry is the mode's landing surface, so the Tags panel is SECOND |
| P02's keyboard bar, as tested | `keyboardpass.test.mjs` tabs from the top in the default mode and `keyboardflow.test.mjs` walks the Annotate flow; **neither reaches a Document-mode panel**, so the panel needs its own keyboard reader |

**Exit criterion 1, read at phase open:** "corrected end to end without leaving nib" needs the user to SEE
what is wrong inside nib. With neither 7.3 t1 nor 7.5 t1 checkable, a user fixing alt text or header
scope has to leave for veraPDF to learn it was needed. **Decided (rung 2, reversible): the checker gains
those two rules in this phase**, validated by the oracle like every other — a narrow addition to
`/pending 486`'s coverage, not its resolution.

**`/plan-review` does NOT fire**: no wire format between machines, no stored nib format, no network, no
credential. **`/deepdive` does not fire at phase open**: the seam it would read — mutating a tree another
producer wrote (inline elements, role maps, shared `/A` arrays, MCR kids) — is S02's and S03's step zero,
measured against LibreOffice's trees with `checkStructConsistency` as the post-condition.

**Firmed slices:**

#### P09.S01 — read an existing tree as reviewable values *(done 2026-09-14, v1.129.85)*
**Amended at step zero:** the route moves to S06. `published.test.mjs` fails every json-tagged server
shape with no named reader, and this one's reader is the panel — P08.S06b+c's reason, again. S01 is the
`pdfops` door (unexported until its route calls it; the zero-caller scan covers exported names).
Scope: a read door (its route, `GET /api/tags/tree`, ships with S06): each element's id, type (and the standard type
its RoleMap resolves to), page, text (its MCIDs' runs through S02 of P08's reader), `/Alt`, a `TH`'s
`/Scope`, and its kids in order. Refs D10.
Acceptance:
- Every element of every LibreOffice corpus tree is represented, in `/K` order, text matching the truth
  reader; the table and figure fixture's `TH` scopes and `Figure` alt read back.
- An element the editor cannot address (written inline, with no object number) is REPORTED, not dropped.
- The route is read-only — the open document byte-identical, asserted.

**(build, 2026-09-14, v1.129.85.)** `readStructureView` in `internal/pdfops/structview.go`.

| measured | result |
|---|---|
| ODT as the table-and-figure source | the same shape LibreOffice's HTML import gave — `Table › TR › TH` (`/Scope /Column`) / `TD`, `Figure` with `/Alt` — and a real `H1`; 21 elements, **0 inline**. `.html` is not an office extension nib converts, and adding one to test a reader would widen what users can open, so the fixture is an ODT through `odtDocument`, which gained the table/draw/svg/xlink namespaces and embedded pictures |
| round trip | every element of the four truth-corpus documents and the table-and-figure document, in `/K` order, object numbers and kids matching the model; text read out of the view by the truth reader's own rule equals `readTruth` block for block |
| table and figure | standard types `Document H1 P Table TR TH P TH P TR TD P TD P TR TD P TD P P Figure`; both `TH` scopes `Column`, no other element a scope; the figure's alt "A grey square", no other element an alt |

- **Pin — `standardRole` is production's now.** The truth reader and the view both resolve through it;
  a test-only copy beside a production one is two answers to what a custom type means (ADR-009).
- **Pin — a third run-reader exemption.** The view reads runs to match MCIDs to text, the truth reader's
  way, and groups nothing; named in the grouping guard at the site with that reason.
- **Pin — a `/Scope` counts only under `/O /Table`.** An attribute object is owned; the same key under
  `Layout` means nothing about header cells.
- **Pin — an element with no `/Pg` takes its first content's page**, directly (an MCR's own `/Pg`) or
  through a kid. LibreOffice names `/Pg` everywhere, so only a hand-built fixture reaches it.
- **Pin — unexported until S06.** The zero-caller scan covers exported names; the route, its reader and
  the exported door land together.

Twelve mutations red, each against its own assertion: the attribute-array branch, the owner check, alt
decoding, kid order, the parent index, the unaddressable count, kids' text, the marked flag, the role map,
both page fallbacks, and a swallowed no-tree error. Three survived the first round — the owner check and
both page fallbacks — and each got a fixture. The slice gate does not fire (`pdfops` only); tiers 2–3 not
run for this slice (no web change).

#### P09.S02 — dictionary edits: retype, reorder, re-parent, alt text, header scope *(done 2026-09-14, v1.129.86)*
Scope: `pdfops` edits over an existing tree that touch no content stream: `/S`, a kid's position within
its parent, a kid moved to another parent (both `/K` arrays and `/P`), `/Alt` on any element, `/Scope`
on a `TH`. Refs D8.
Acceptance:
- Each edit leaves `checkStructConsistency` empty on every corpus tree, asserted per edit kind.
- The stripped-`/Alt` fixture with alt restored clears veraPDF 7.3 t1; the stripped-`/Scope` fixture with
  scope restored clears 7.5 t1; neither adds a clause.
- An edit naming an element the tree no longer has is refused as stale.

**(build, 2026-09-14, v1.129.86.)** `applyStructEdits` in `internal/pdfops/structedit.go`.

| measured | result |
|---|---|
| how LibreOffice stores what the edits touch | every `/A` DIRECT and unshared (18 dicts, 2 arrays, 0 references), every `/K` an array; other producers may do neither, so both the shared-reference and single-entry forms are driven by hand-built trees |
| each edit on LibreOffice's own trees | retype, alt, reorder under the root, re-parent into the list, a two-edit batch, and a scope change: each visible in the re-read view, every marked element's type and text unchanged, the consistency invariants clean |
| veraPDF on the table-and-figure document | original fails `7.1 t8`, `7.1 t10`; with `/Alt` and `/Scope` stripped it adds **7.3 t1** and **7.5 t1**; restored through three edits it fails `7.1 t8`, `7.1 t10` again — both cleared, nothing added |

- **Pin — the ParentTree is not touched.** A slot names the element that owns an MCID, and no edit here
  changes ownership; a move writes the two walk directions, the parent's `/K` and the element's `/P`.
- **Pin — a scope edit never writes through an attribute object.** The Table attribute object is copied
  onto the edited element; every other attribute object, references included, is kept as written.
  Driven by two header cells sharing one indirect object.
- **Pin — a position counts ELEMENT kids.** MCIDs interleaved in a `/K` keep their places.
- **Pin — a batch re-reads the tree before each edit**, so a second move resolves "its current parent"
  where the first put it.
- **Pin — retype chooses a STANDARD type** (ISO 32000-1 §14.8.4); a custom name means something only
  through a RoleMap, and a correction names what it means.
- **Pin — a stale edit is its own sentence.** It is `ErrTagsStale` to `errors.Is` (so 409 at S04's route)
  and speaks about a tree; the first cut reused P08's "the proposal does not match — propose again".
- **Finding, fixed in the slice — `checkStructConsistency`'s defects were compared by their SENTENCE, and
  the sentence names the element's type.** Retyping an element in a tree that already had a defect
  changed that defect's text, so the edit read it as a new defect and refused itself. Defects now carry
  a key of what is broken (page key, MCID, owning object) and never a type; the edit compares keys.
- **Pin — the refusal of a defect an edit ADDS has no reachable stimulus.** None of the four edits can
  change MCID ownership or a page's key, so no test can make an edit produce a new defect. It stays as a
  backstop over the one invariant set the edits are argued not to touch, and is recorded as unproven
  rather than implied covered.
- **Pin — the tag source (D4) is not changed.** A corrected tree keeps the tier of the producer that wrote
  it; ADR-031's claim is untouched.

Twenty-one mutations red. First round, fourteen of seventeen: the type check, the retype write, alt
escaping, alt removal, the TH-only and value checks, the attribute copy, the single attribute object kept,
scope removal, the cycle check, `/P`, the indirect-`/K` write, and the index. Three survived — MCIDs counted
as elements, the per-edit re-read, and the pre-existing-defect excuse — and each got a fixture; the third
found the finding above. Then the stale error's `Is`, its wording, and sentence-keyed comparison. The slice
gate does not fire (`pdfops` only).

#### P09.S03 — mark an element's content as an artifact *(done 2026-09-14, v1.129.87)*
Scope: the one edit that touches content streams — its MCIDs' `BDC … EMC` become `/Artifact BMC … EMC`,
its ParentTree slots are freed, and the element leaves its parent's `/K`.
Acceptance:
- The bracketed bytes are unchanged (the P08.S07 invariant, reused); the ParentTree no longer names the
  element; `checkStructConsistency` is empty; veraPDF adds no clause.
- An element whose content is drawn inside a form XObject is refused, as P08's commit refuses it.

**(build, 2026-09-14, v1.129.87.)** `artifactElement` in `internal/pdfops/structartifact.go`, applied as
`editArtifact` through S02's batch.

| measured | result |
|---|---|
| how LibreOffice marks content | inline `/Tag<</MCID n>>BDC … EMC`, one sequence per leaf; the figure's sequence draws an IMAGE `Do` (`/Subtype /Image`), not a form |
| a door that finds a sequence by MCID | **none that fits**: `watermarkArtifactSpans` finds only `/Artifact <<…>> BDC` openers, and a figure has no text runs, so run spans cannot find it. The run walker already resolves each `BDC`'s MCID (inline or `/Properties`) and sees operand starts and form descent |
| a paragraph and a whole list artifacted on LibreOffice's tree | gone from the tree with every descendant; their ParentTree slots empty; their text still drawn, now as artifact runs; the page's tokens identical but for the openers (tokenized independently of the writer's spans) |
| the figure artifacted | no sequence opens with its MCID; the image `Do` still drawn |
| veraPDF, a paragraph and the figure artifacted | fails `7.1 t8`, `7.1 t10` — the original's own; nothing added |

- **Pin — the run walker records every MCID-carrying sequence** (`markedSeq`: opener span, `EMC` span,
  in a form, draws a form), and the artifact edit reads that — one reading of `BDC` for text, figures and
  the writers. `drawForm` now answers whether the XObject IS a form, walked or not (a self-drawing or
  too-deep form still makes a sequence around it form content).
- **Pin — only openers change.** `/P <</MCID n>> BDC` becomes `/Artifact BMC`; the `EMC` and the bytes
  between are untouched.
- **Pin — six refusals, each driven:** a sequence around a form `Do`; one inside a form's own stream;
  an MCR into a form stream (`/Stm`); an element describing an annotation (`OBJR`); a sequence enclosing
  another element's (closed or not); an MCID the page does not draw; and the last element of the tree
  (a claim over an empty tree, ADR-031).
- **Pin — emptying a slot never creates a ParentTree.** `clearParentTreeSlot` returns when there is none.
- **Pin — `removeFromParent` is the one way an element leaves where it is**, shared by the move and the
  artifact edit.
- **Harness note — the first mutation run was killed by the OS for memory** (each round compiled
  `pdfops` and ran LibreOffice and veraPDF), with a mutation still on disk; it was restored from the
  pre-run backup and verified byte-identical before anything else ran. The rerun restored in `finally`
  and left the veraPDF test out of the per-mutation set; the one mutation only it would have caught got a
  fixture instead.

Twenty-one mutations red. Five survived their first run and each got a fixture: the in-form refusal (no
sequence inside a form's own stream), the unclosed-enclosure branch, "any later sequence counts as
enclosed" (no artifacted element was followed by another's sequence), the no-ParentTree guard (the
positive fixture's page named no key, so the path was never reached), and the per-stream shrink of the
open-sequence stack (no form left a sequence open). The slice gate does not fire (`pdfops` only).

#### P09.S04 — the routes *(done 2026-09-14, v1.129.88)*
Scope: `POST /api/tags/edit` — a batch of S02/S03 edits against the tree S01 served — through
`commitMutation`, so undo holds. Refs ADR-001, ADR-008.
Acceptance:
- A signed document is refused at the server door; an edit against a tree that changed is 409.
- One batch is one undo step, and `POST /api/undo` restores the prior bytes.

**(build, 2026-09-14, v1.129.88.)** `pdfops.EditStructure` (`structedit.go`) and `POST /api/tags/edit`
(`internal/server/tags.go`).

| measured | result |
|---|---|
| a batch through the route | a retype and an alt text on a committed proposal reach the document; ONE `POST /api/undo` takes both back, and a second takes back the commit (the tree is gone) |
| refusals at the route | a signed document 409 naming the signature, bytes unchanged; an element the tree does not have 409; an edit kind that is not one, a non-standard type, and an empty batch 400; a document with no tree 409 — nothing written in any case |
| a move with no `index` | appends — the route translates an absent position, where Go's zero value would have put the element first |
| the standing guards | law 2's census drives `EditStructure` (an alt text on the fixture's first addressable element) and measures `carried`; the `resolveDoc` count 28 → 29; zero-caller satisfied by the route; `published.test.mjs` 4/4 (the request shape is anonymous, as the commit's is) |

- **Pin — the route reads the tree from the document, and so do its tests.** The GET tree route and its
  reader land with S06; until then the server tests find element object numbers by parsing `/api/pdf`,
  and compare after an edit by position and type, since a write may renumber.
- **Pin — a document with no tree is STALE (409), not malformed.** The reviewer was shown a tree.
- **Finding, fixed in the slice — the no-tree refusal spoke about a proposal.** It wrapped `ErrTagsStale`
  with `%w`, and that sentinel's own text is P08's "the proposal does not match the document any more";
  the mutation probe's output showed it. It is `staleEdit` now, S02's tree-worded stale error, and the
  route test refuses a 409 that mentions a proposal.
- **Pin — the unknown-kind check is an equivalent mutation.** Without it a zero kind still reaches
  `applyStructEdit`'s default, `ErrTagsReview`; the check is kept for the sentence it gives.

Six mutations red: the signed check, stale answered 400, an absent index read as 0, the no-tree mapping,
`commitMutation` bypassed (caught by the routing test), and the proposal wording on a missing tree. The
slice gate does not fire: `internal/server`'s tag routes, not its session, ceremony, delivery or
discovery paths.

#### P09.S05 — the checker sees what the editor fixes: 7.3 t1 and 7.5 t1 *(done 2026-09-14, v1.129.89)*
Scope: two `uacheck` rules, registered like the other fifteen. Refs P07, `/pending 486`.
Acceptance:
- The oracle agrees with veraPDF on the LibreOffice fixture and both stripped copies, per clause, both ways.

**(build, 2026-09-14, v1.129.89.)** `internal/uacheck/rules_semantic.go`; a tree walk (`structNodes`) and a
Table-attribute reader in `structure.go`, the checker's own reading, not `pdfops`' model.

| measured | result |
|---|---|
| veraPDF's 7.3 t1 | object `SEFigure`; test `(Alt != null && Alt != '') \|\| ActualText != null` — implemented as written, through the role map |
| veraPDF's 7.5 t1 | object **`SETD`** — every data cell, not every header; test `hasConnectedHeader != false \|\| unknownHeaders != ''`, "connected" decided by veraPDF's own algorithm |
| the first rule, a grid of Scope directions, against veraPDF on hand-built tables | **wrong in five shapes**: veraPDF passed row-scoped headers above cells, a column-scoped header before one, headers below, a row header after, and a `Both` corner beside an unheaded cell — nib failed all five |
| which cells veraPDF fails (object paths of 11 more tables) | **no statable rule**: in unscoped tables one cell fails and not its identically placed twin (LibreOffice's stripped table: `(1,0)` fails, `(2,0)` does not); the failing corner moves shape to shape; a `Headers` attribute on one cell stops the other failing |
| what never failed | **a table whose every `TH` has a Scope (Row, Column or Both), or that has no `TH`** — every such shape measured; each is a Pass case in `tableCases`, re-measured by the standing agreement test |
| the oracle | 459 of 459 (document, clause) pairs over 27 documents; the corpus gains LibreOffice's table-and-figure document, its `/Alt`- and `/Scope`-stripped copies, and a copy with row headers written through `EditStructure` |

- **Pin — 7.5 t1 answers only what was measured.** Every header scoped, or none present: Pass. An unscoped
  `TH` over a data cell that names no headers: **CannotCheck**, naming that header cell — the correction is
  the same whichever cell veraPDF picks. A span over such a cell, and a `TD` outside any table, likewise.
  One `knownCannotCheck` row records the stripped LibreOffice table. Rung 2, reversible: a statable model
  of veraPDF's cell choice would turn those into Fail.
- **Pin — the hand-built cases are a standing oracle reader.** `TestTheFigureAndTableCasesAgreeWithVeraPDF`
  runs veraPDF over every unit case on its own clause, and requires at least 20 of them settled either
  way — CannotCheck agrees with everything, and a rule that answered it everywhere would pass otherwise.
  It is what found the first rule wrong; the unit expectations alone had agreed with nib.
- **Pin — the corpus floor counts LibreOffice's documents by prefix.** Its generated floor stays 22.
- **Amendment to S07** (below): with the stripped `/Scope`, 7.5 t1 reads *Nib could not check* and names
  the header cell, not *fails*; after the scopes are set it passes. The criterion is the same — the problem
  is visible and correctable in nib — and S07 asserts the verdict nib gives.
- **Count claims** move from 15 to 17 of 106 in the README (and its standing reader), `door.go`,
  `rules_catalog.go`, `uacheck.go` and the oracle's comment; P07's "15" stays where it records what was
  measured then.

Sixteen mutations red, each against its own assertion: the `/ActualText` branch, an empty alt, the figure
role map, no figures as Pass, a scope name that names no direction, `Headers` naming, the bare-cell
condition, unscoped detection, span detection, a span over cells that all name headers, `THead`/`TBody`,
the stray `TD`, no data cells as Pass, the cell role map, the attribute owner check, and the attribute array.
Two probes were first written so they did not compile and were redone; two survived and each got a case.

#### P09.S06 — the Tags panel and the Reading Order view
**(step zero, 2026-09-14 — SPLIT into three, as P08.S06 was.)** A read route and its reader, element
geometry, a sidebar panel with an ARIA tree, five keyboard edits and a reading-order overlay are too much
for one slice to probe honestly.
- **P09.S06a — the read surface** *(done 2026-09-14, v1.129.90)*: `pdfops.ReadStructure` with each
  element's box, `GET /api/tags/tree` (S01's route, moved here with its reader), and a panel showing the
  active document's tree as an ARIA tree, the focused element outlined. Tiers 1 and 2.
- **P09.S06b — the edits**: retype, move, alt text, scope and artifact on the selection, keyboard-only,
  through S04's route; the panel's tier-3 keyboard reader and the report changing after an edit and back
  after undo.
- **P09.S06c — the Reading Order view**: the overlay numbering elements on the page.

Scope: the sidebar panel second in `SIDEBAR_FOR.edit`: the tree (ARIA tree pattern), the selected
element outlined on its page, and retype / move / alt text / scope / artifact on the selection; a
reading-order overlay numbering elements on the page. Refs D10, ADR-018, ADR-020.
Acceptance:
- Every edit reachable keyboard-only, at tier 2 and tier 3, with the panel's own keyboard reader: no trap,
  focus always visible, and the flow completes (P02's bar).
- The report's clauses change after an edit and change back after undo.

**(P09.S06a build, 2026-09-14, v1.129.90.)** `pdfops.ReadStructure` (`structview.go`), `GET /api/tags/tree`
(`internal/server/tags.go`), the Review Structure Tree panel (`web/app.js`, `#tagtree`).

| measured | result |
|---|---|
| where the panel's state lives | ONE shared panel loaded for the active document, not per-view DOM like the outline: it follows a load (`setDocumentFromServer`, beside `buildOutline`), a switch (`repaintForActiveView`) and its own header |
| an element's box | the union of the runs under its MCIDs (P08's estimate, 0.85 em up, 0.25 em down) and its kids' boxes ON ITS OWN PAGE, with that page's MediaBox; LibreOffice's cells sit inside their table's box and Name left of Qty; the figure, which draws no text, has none |
| the route | byte-identical after a read; its top level matches the document's `/K`; every kid names its parent; every text element's box inside its page; a document with no tree answers `tagged: false` with an empty list |
| the panel at tier 2 | second in Document mode, not its landing surface; one tab stop; levels; Down/Up/Home/End/Right-to-first-kid/Left-to-parent; focus moves selection and the tab stop and takes the viewer to the element's page, a boxless figure included; an untagged document says so and names Tag structure…; an undo re-reads; a switch re-reads; a late answer for the document left behind is dropped |
| the standing guards | `published.test.mjs` (both shapes read), the observables scan (`pdfops.StructureTree`), `resolveDoc` 29 → 30, ids, tablist and modes unchanged and green; jsdom files 65 → 66 |

- **Pin — pinned once, after the whole answer.** The first cut checked the sequence and owner after the
  fetch AND after the body; a probe removing one survived because the other held. One check, after both
  awaits, which the late-answer test makes red.
- **Pin — the handler trusts the door for non-nil kids.** A second nil guard in the handler could not be
  made to fire; it was removed and `ReadStructure`'s test holds the property.
- **Pin — the outline is on the element's first page.** A section spanning pages is outlined around its
  first-page content; a union across pages would outline a region of page 1 that page 2's text occupies.
  Driven by a two-page fixture after the one-page corpus let the filter's removal survive.
- **Pin — no tier 3 in this part.** The outline's position on a rendered page and the panel's real Tab
  order are S06b's tier-3 reader, which exercises them while editing.

Twenty mutations red. Go: the box union's edges, kids' boxes, the same-page filter, the page box,
nil elements and nil kids from the door, a null list from the handler. Client: the panel first, the header
hook, `aria-level`, Right, Left, Home, End, the roving tab stop, a figure's page move, the untagged summary,
the reload hook, the switch hook and the pin. Four survived their first run — the same-page filter, the pin
(duplicated), and both refresh hooks — and each got a test; the handler's nil guard was removed as
unfalsifiable. The slice gate does not fire (`internal/server`'s tag routes).

#### P09.S07 — end to end, and the exit criteria
Scope: tier 3 — open the LibreOffice fixture with alt and scope stripped and headings mapped to `P`, see
7.3 t1 fail and 7.5 t1 read *could not check* naming the unscoped header (S05's amendment) in nib's
report, correct all three in the panel by keyboard, and see both clauses pass, without leaving nib.
Acceptance:
- veraPDF agrees on the corrected document; undo returns each clause.

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
