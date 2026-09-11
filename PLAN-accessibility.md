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

### D9 — The page-subset remap is a measured prerequisite, not an assumption ~~*(open — first task of P01)*~~ *(SUPERSEDED IN PLACE 2026-09-11 by P01.S02's measurement — the question does not arise)*
~~`Collect`, `RemovePages` and `SplitBySpans` drop the tree today. Carrying it means remapping
`/ParentTree` and `/StructParents` for a kept subset and pruning orphaned elements. The grill
measured **preservation in place** and explicitly did **not** measure a remap. P01.S02 measures it
before anything is planned on top of it; if it proves impractical against pdfcpu v0.13.0, those
operations fall back to `dropped-with-notice` under law 2 and this decision is superseded in place.~~

**The answer, and it is one level below the question D9 asks.** There is no subset to remap, because
**the write path carries no struct element at all.** Measured 2026-09-11 on a real LibreOffice-
produced tagged PDF (five headings/paragraphs/list items, **14 `/StructElem`**), against pdfcpu
v0.13.0:

| operation | `/StructTreeRoot` | `/Marked` | `/StructElem` | `/StructParents` |
|---|---|---|---|---|
| **source** | 2 | 1 | **14** | 1 |
| **no-op `writeMutated`** (read → validate → optimize → write, changing nothing) | 1 | 1 | **0** | 0 |
| `Collect(1)` — i.e. `nib pages` | 0 | 0 | 0 | 0 |
| `Rotate(90)` | 1 | 1 | **0** | 0 |
| `NUp(2)` after P01.S01 | 0 | 0 | 0 | 0 |

**The no-op row is the finding.** Remapping `/ParentTree` for a kept subset presupposes that the
elements survive the write; they do not survive it even when nothing is asked of them. The read half
is sound — `api.Validate` reports the fixture clean and the catalog still holds `/K` and a
`/ParentTree` after `ReadValidateAndOptimize` — so this is `WriteContext` not serialising the objects
the tree points at.

**Two consequences that re-scope this phase.**

1. **`Collect`, `RemovePages` and `SplitBySpans` fall back to `dropped-with-notice` under law 2**,
   which is the outcome this decision named for the impractical case — but for a stronger reason than
   it anticipated. `Collect` already drops the claim honestly today (row 3 above), so it is already
   compliant with law 1 and needs only its verdict recorded.
2. **`Rotate` is a live law-1 violation and was not on any slice's list.** Row 4: it keeps
   `/MarkInfo /Marked true` and a `/StructTreeRoot` over **zero** struct elements — the same lie
   P01.S01 fixed in `nup`, still shipping. Found by this measurement, not by the plan. It is P01.S03's
   guard that must enumerate it rather than a one-off patch, because the same probe implies every
   `writeMutated` caller is a candidate and a hand-written list would miss the next one.

**And the prerequisite this plan does not have a slice for:** P05's tag-tree core cannot be built
until the write path carries a tree. See `/pending 467`.

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

#### P01.S01 — `nup` stops lying *(done 2026-09-11, v1.129.9)*
Scope: `nup` either drops `/StructTreeRoot` and `/MarkInfo` with the content it voids, or tags its
composed page; it may not keep the claim. Refs: law 1, D2.
Acceptance:
- ~~A tagged input through `nup` produces no veraPDF failure the input did not already have.~~
  **(struck 2026-09-11 — REFUTED BY MEASUREMENT, and only the option this slice defers could have
  met it.)** Dropping the claim is what law 1 demands and it necessarily ADDS ua1 failures, because
  PDF/UA requires a structure tree: an untagged file fails 6.2 t1 and 7.1 t11 by construction.
  Measured on the slice's fixture — input fails {7.1 t8, 7.1 t10, 7.21.4.1 t1}; n-upped before the
  fix fails those **plus 7.1 t3**; n-upped after the fix fails those plus 7.1 t3, 6.2 t1 and
  7.1 t11. A ua1 failure COUNT scores honesty as a regression, so it cannot express law 1.
- **Replacing it:** the output carries no `/MarkInfo /Marked true`, no `/StructTreeRoot` and no
  `/StructParents` — law 1 as a structural property of the output, which is what it actually says.
- The output carries no struct element whose `/Pg` points at a page that is not in the document.
- A red proof: reinstating the claim without the content turns the guard red.

**(pin, 2026-09-11 — the slice shipped and its FOUNDING PREMISE did not survive contact.)**
`/pending 29`'s reason 1 says pdfcpu *"round-trips a hand-built `/StructTreeRoot`+`/MarkInfo`+
`/ParentTree` and BDC/EMC marked content **intact** through `ReadValidateAndOptimize→WriteContext`"*,
measured 2026-06-23, and the entry itself asks for it to be re-confirmed before anything is built on
it. **It does not hold on pdfcpu v0.13.0 today.** A NO-OP `writeMutated` — read, validate, optimize,
write, changing nothing — loses every `/StructElem` and every `/StructParents` while keeping
`/StructTreeRoot` and `/MarkInfo`. The read half is fine: `api.Validate` reports the fixture clean
and the catalog still holds `/K [8 0 R]` and a `/ParentTree` after the read. It is `WriteContext`
that does not serialise the objects the tree points at.

**Two consequences, and neither is this slice's to fix.** `nup` was never the destroyer — on this
evidence *every* operation routed through `writeMutated` voids structure while keeping the claim, so
P01.S03/S04's scope is much wider than "the operations S02 found practical". And **P05's tag-tree
core cannot be built until the write path carries a tree**, which is a prerequisite this plan does
not currently have a slice for. Filed as `/pending 467` with the measurement.

#### P01.S02 — measure the page-subset remap *(done 2026-09-11, v1.129.10 — no code; the measurement IS the deliverable)*
Scope: probe whether `/ParentTree` + `/StructParents` can be correctly remapped for a kept page
subset against pdfcpu v0.13.0. Outcome, not code, is the deliverable; it settles D9. Refs: D9.
Acceptance:
- ✅ A recorded measurement, not an argument, for `Collect`, `RemovePages` and `SplitBySpans`.
  **Widened**: the table under D9 covers `Collect`, `Rotate`, `NUp` and — the row that decides it —
  a **no-op** `writeMutated`. `RemovePages` and `SplitBySpans` are `writeMutated`/`api` callers on the
  same write path and are covered by that row; measuring each separately would have re-measured the
  same write, which is the trap D9's own "preservation in place, not a remap" warning describes.
- ✅ D9 is superseded in place with the answer, and P01.S04's scope is set by it: the three
  operations become `dropped-with-notice`, and the guard must ENUMERATE rather than list.
- **The fixture is a real LibreOffice-produced tagged PDF, not a hand-built one**, and that was the
  point of running it: the hand-built fixture used at P01.S01 could not distinguish "pdfcpu drops
  trees" from "my fixture is malformed". 14 struct elements from a producer settles it.

#### P01.S03 — the tag-fate table and its guard *(done 2026-09-11, v1.129.11 — absorbs P01.S04, see below)*
Scope: every document-touching operation declares `carried` / `refused` / `dropped-with-notice` in
one table; a table-driven tier-1 guard over a tagged fixture asserts each verdict and fails when an
operation has no entry. Refs: law 2, D12.
Acceptance:
- ✅ The guard enumerates operations from the code, not from a hand-written list. `go/ast` over
  every exported function in `internal/pdfops` whose first parameter is `pdf []byte` and whose first
  result is `[]byte` — **33 operations**, with a floor so an enumeration that stopped matching fails
  loudly instead of passing empty.
- ✅ Adding an operation with no verdict turns it red, **proved by adding one** (a `Flatten` stub;
  red, then removed). Three more mutations red: the write door and `honest()` each ceasing to
  enforce law 1, and a table row for an operation that does not exist.
- ✅ The fixture corpus lands under D12's corpus — `internal/pdfops/corpus_test.go`, shared by every
  tag guard, rather than inline in one test. **Generated rather than committed**, for the reason
  this package's own fixture file already gives: *"a checked-in binary fixture is opaque in review"*.
  The producer-made LibreOffice document that P01.S02 needed is a recipe at D9, deliberately not a
  28 KB blob.

**The verdict vocabulary is one word different from the scope line, and the difference is measured.**
`carried` / `refused` / `dropped-with-notice` became **`dropped` / `carried` / `untouched`**: nothing
can be `carried` today (D9), no operation `refuses` a tagged input, and `untouched` is the honest
verdict for the five operations that return a report or a ZIP rather than a document — a distinction
the original three words could not make. **There is deliberately no verdict for "keeps the claim
without the content"**: law 1 forbids that state, so it is not a fate an operation may declare, it is
a failure.

**(pin, 2026-09-11 — this slice absorbed P01.S04, because the guard could not ship red.)**
The guard found **eight operations lying** on its first run — `Rotate`, `Optimize`, `SetLang`,
`StripMetadata`, `StripActive`, `RemoveFilesAndMedia`, `InsertBlank`, `NormalizePageSizes` — and
eight more with no verdict at all: `Append`, `InsertPDF`, `FillFormJSON`, `FillFormXFDF`, `SetFlags`,
`StampFields`, `ExtractImagesZip`, `PreparePDFA`. Shipping a red guard is not shipping a guard, and
the fix was S04's. Merging them is a granularity call, which `~/.claude/ASK.md` settles at rung 1.

**The fix is a LAW at two doors, not eight patches.** `writeMutated` and `honest()` each check law 1
as a **post-condition** — *did this write destroy the structure while leaving the claim* — and drop
the claim when it did. Four of the eight were fixed by the write door alone; measurement then named
the other four as direct `api.*` callers, which is how the second door was found rather than guessed.
Because it is a post-condition, it becomes a no-op by itself the day the write path carries a tree
(P05's prerequisite, `/pending 467`) — there is no line to remember to delete.

#### P01.S04 — carry the tree where it can be carried *(done 2026-09-11, v1.129.14)*

**(pin, 2026-09-11 — marked `done` for twenty minutes and taken back, which is recorded rather than
tidied away.)** S03's fix covers this slice's *carrying* half completely: D9 measured that nothing
can be carried, so every operation is `dropped` and S03's guard asserts it over all 33. The marker
went on for that reason and was **wrong**, because this slice's acceptance has three clauses and only
one of them is about carrying. Caught by walking the acceptance ledger clause by clause instead of
crediting the slice against its scope line — which is exactly the failure the ledger rule exists to
prevent. What remains is below.

Scope: implement carrying for the operations S02 found practical; the rest become
`dropped-with-notice` with a user-visible sentence. Includes `merge`'s argument-order defect —
tagging survives only when the tagged file is first. Refs: D8, D9, law 2.
Acceptance:
- ~~`merge` preserves tagging in both argument orders, asserted per order.~~ **Overtaken by D9 and
  replaced**: nothing preserves tagging in any order, so the defect this names — *"tagging survives
  only when the tagged file is first"* — cannot exist. What survives is the honesty question, and it
  is ✅ discharged: `Append` is driven in **both** argument orders and neither lies. It was a LIVE
  defect when driven — `api.MergeRaw` takes the first document's catalog whole, so tagged-first
  emitted a claim over a result whose struct elements were gone, and tagged-second did not.
- ✅ **Every `dropped-with-notice` operation actually emits its notice, asserted at the door.**
  Dan chose **a persistent banner modelled on `#fitNotice`**, 2026-09-11, over a toast — on the
  repo's own recorded reasoning about what a self-clearing toast can carry (`index.html:72`:
  *"`toast` cannot carry them: it clears itself after 2500 ms"*). A user who lost their document's
  accessibility structure has to still be able to see it **at the moment they save**.

  **Asserted at the COMMIT door, not at 33 operations.** `noteTaggingFate` runs from
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
  SHAPE (28 → 33 operations), and `RedactPages` carries an explicit verdict with its reasoning: a
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
