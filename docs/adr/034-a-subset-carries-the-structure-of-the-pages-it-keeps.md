# ADR-034 — a page subset carries the structure of the pages it keeps, and a subset feeding a composition does not

**Status:** accepted
**Date:** 2026-09-16
**Context:** `PLAN-ua-coverage.md` P02.S04b, D5. Extends ADR-031 and ADR-009; rests on ADR-032.
**Applies:** `pdfops.Collect`, `RemovePages` and everything built on them (`DuplicatePage`, `Booklet`,
`SplitBySpans`, `SplitByBookmarks`), the non-carrying door `collectWithoutStructure` and its callers, and
`carryIsComplete`, which `NUp` shares.

## Decision

**A subset carries the structure of the pages it keeps.** `Collect` and `RemovePages` prune the source
structure tree in place, in the source context where object numbers are stable (ADR-034 exists because
P02.S04a moved page selection there for this purpose), and the catalog's `/StructTreeRoot`, `/MarkInfo`, a
rebuilt `/Metadata` and a `/DisplayDocTitle`-only `/ViewerPreferences` are re-added **on top of** the
default-deny allowlist, only when the carry succeeded. Six rules make it true:

1. **An element dies when it loses every kid, never because its own `/Pg` died.** A surviving element whose
   `/Pg` names a dropped page loses that key; its MCID kids went with the page, and an MCR or OBJR kid carries
   its own `/Pg` and may be on a page that survived.
2. **An OBJR's liveness is its `/Obj`'s, not its `/Pg`'s.** `/Pg` is optional on an OBJR, and the object is
   what re-anchors.
3. **`/ParentTree` rows are rebuilt from the claimants the OUTPUT has, renumbered `0..n-1` in output order**,
   with `/ParentTreeNextKey` set to `n`; a claim on a row that did not survive is deleted from the claimant.
4. **A repeated page gets its own key and a deep-cloned subtree**, for the page's `/StructParents` and for each
   cloned annotation's `/StructParent`, with `/Pg` and an OBJR's `/Obj` repointed at the copy.
5. **The gate reads the bytes it wrote, and an incomplete carry falls back to the honest loss.** A refusal
   costs nothing and needs no rollback: the four keys are simply not restored, so the pruned tree is
   unreachable and pdfcpu leaves it out. The carry also refuses what neither codified predicate can see — a
   prune that leaves nothing anchored.
6. **A subset whose result is then COMPOSED does not carry.** `splice` (behind `InsertPDF`, and behind
   `replacePage` and so `SplitPage`/`SplitRegions`), `normalizePage`, `SplitRegions`' own page selection and
   `SplitPage`'s tile selection route through `collectWithoutStructure`, each exemption named at its site. The
   two file splitters compose nothing after their subset and therefore carry.

**Four losses are declared rather than solved**, because each is a way a removed page or an embedded file
re-anchors: `/IDTree` is dropped outright, every carried element's `/AF` is dropped, `/T` goes from any element
the prune changed, and a duplicated page's structure reads item by item alongside the original's rather than as
a second block (`/pending 529`). `/Alt` and `/ActualText` are KEPT and that is the honest limit of what a
subset can promise — a description written about a section spanning removed pages still describes that whole
section, and a truncated `/ActualText` would be a false statement rather than a lossy one.

**A subset PRESERVES its input's fate rather than setting it.** On a fully-described document the output is
`carried`; on a partially-described one — the `api.MergeRaw` shape every ceremony document takes — it is
`partial`; where it keeps only pages nothing describes, the carry refuses and the claim goes. A subset does not
*make* a partial document. What the old behaviour did was launder one by destroying the whole live tree.

## Why the prune is the decision and the carry is not

Keeping the four catalog keys and pruning nothing measures **`carried`** — the census's best verdict — while
being a lie. Measured on the census document, `Collect(["1"])` that way comes out with all 364 elements, 217
completeness defects, and **229,859 decoded stream bytes in a one-page output** against 91,481 today: the seven
dropped pages' content streams are still in the file, reachable through the surviving elements' `/Pg`. pdfcpu
writes by reachability, so that is not untidiness — it is *"delete page 4"* shipping page 4's text.

**Neither `fate()` nor `orphaned()` can see it.** They ask whether the tree anchors to *anything*, which a tree
anchoring to most of what it describes passes. So the reader for this decision is
`structureCarriedCompletely` (ADR-031's census is a necessary condition only), and
`carryIsComplete` gained the condition that catches the whole class rather than a shape: **no `/Type /Page`
object outside the page tree.** Every re-anchoring defect this package has had was something that still NAMED a
dropped page — a field's `/Kids`, a link's destination, a surviving named destination, an element's `/Pg`, an
`/IDTree` entry, an OBJR's `/Obj` — each found and fixed separately. That condition asks what all six answer,
needs no list of keys, and cannot go stale as new ones appear. It **dereferences** rather than type-asserting,
because pdfcpu holds an object-stream page as a lazy object until something resolves it, and a detector written
the other way reported a document with a dropped page in it as clean.

## Why an element's own `/Pg` cannot decide its death

The first design said it could. Measured: LibreOffice writes `/Pg` on **523 of 523** elements including the root
`/Document` (`/Pg` = page 1) and a `/Table` whose `/Pg` is page 3 while its rows are on 3–5. Dropping page 1
under that rule leaves **0 elements of 523**, and it does so on **5 of 7** real multi-page tagged documents
available — while nib's own census document is bit-for-bit identical under both rules in every selection. The
one fixture the repo had could not see it.

## Why a composition must not inherit the carry

`api.MergeRaw` keeps only the FIRST document's catalog and `/ParentTree`, and later documents' pages keep their
own `/StructParents` values. Measured on two tagged documents: after a carried subset of A is merged with B, B's
pages hold B's content and resolve into A's rows — `/ActualText` included, so a reader is handed a positive
false statement about what the page says, where the honest loss states nothing. `CutPage` leaves
`/StructParents` on tiles whose tree it destroyed, which is the same shape one step over. P02.S05 (crop), S06
(the splits) and S07 (the merge graft) each own their half of that question; a carry reaching those doors would
answer it by accident.

## What this does to ADR-031 and ADR-009

**ADR-031's census verdicts change**, and law 1 is untouched: `Collect`, `RemovePages`, `DuplicatePage`,
`Booklet` and `CarryAttachments` move from `dropped` to `carried`, their `pageSetLoss` rows leave
`knownUA1Deltas`, and a subset joins `Append` as a producer of `partial` — which ADR-031 already records as not
enforced, for the reason that still holds. Measured by veraPDF on a conformant document: `Collect`,
`DuplicatePage` and `Booklet` now add **no clause at all** where each previously added all five.
`RemovePages` adds `7.4.2 t1` because its census drive deletes the page carrying the document's only `/H1` —
an honest consequence of the deletion, not a structure loss.

**ADR-009 gets one primitive with two named wrappers** — `selectPages(ctx, keep, carry)` behind `subset` and
`subsetCarrying` — and **one declared exemption**: "who claims a `/ParentTree` key" has a read-side walk
(`parentTreeOwners`, shared with the completeness predicate and the key allocator) and a write-side twin
(`eachParentTreeClaim`), because renumbering must write a claimant's key back and the read side returns
descriptions. A declaration alone does not stop two walks drifting, so a test compares their answers.

**ADR-032 cannot be reinstated by the carry, by construction rather than by ordering.** The packet
`carryTitleFloor` writes is NEW — `buildXMP` over the source's `dc:title` and nothing else — so there is no
identification to survive. Carrying the source packet whole was refused for the same reason `selectPages`
empties `/Info` and clears the trailer's permanent `/ID[0]` six lines away: measured, it shipped `dc:creator`,
`dc:description`, `pdf:Keywords`, `xmpMM:DocumentID` and `xmpMM:History` into every extract. `/ViewerPreferences`
is carried key by key for the same reason — carried whole it brings `/PrintPageRange`, which is page-indexed.

## The cost, measured

A tagged reorder is 2.3–4.0× slower over 8–146 pages (6→14, 18→72, 41→145, 77→294 ms), the output gate is
40–50% of that, and the output is 1.07–1.59× larger, which is what a true tree costs. The ratio is still
climbing past that range — 4.9× at 582 pages, the gate about half of it — because the gate runs three page
sweeps each calling `ctx.PageDict` per page (`/pending 530`). An untagged document pays nothing: the carry is
skipped as a precondition, not a fast path, because `carryIsComplete` answers *false* for an untagged document
and a gate that ran unconditionally would fall back forever.

Two consequences follow from the size and are recorded rather than solved: a full-document reorder's output is
now about 1.0× its input rather than 0.7×, so **ADR-005's byte cap becomes newly reachable on a rearrangement,
after the work is paid for**; and every later undo entry grows by the same factor against ADR-003's pool.
