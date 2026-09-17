# ADR-046 — a page-indexed key is restated, and a two-form key is split by form

**Status:** accepted · v1.138.x

## Context

ADR-043 carried the four catalog keys `/pending 525`'s corpus scan found that were **not**
page-indexed, and named the two it could not take for that reason. This is those two:

- **`/OpenAction`**, 220 of 295 readable PDF_UA-1 catalogs — the second most common dropped key
  after `/Outlines`, and what makes a document open at the place its author meant.
- **`/PageLabels`**, on `pdfops.go`'s *"deliberately NOT carried"* list since the subset was written,
  with the reason stated there: *"a label range names a page INDEX, so carrying it needs the old→new
  index MAP, which is a different instrument from the kept-page SET the rest of this operation
  uses."*

`/pending 524` had already refuted the belief that `/PageLabels` is *"the same shape"* as the
outline. An outline destination names a page **object**, and `selectPages` rewrites the page tree in
place, so a kept page keeps its object number and the carry is a prune with nothing to remap. Neither
of these keys is like that. One is keyed by page index; the other is one key meaning two different
things.

## Decision

### 1. A page-indexed key is RESTATED against the output's positions, never remapped

`/PageLabels` is rebuilt rather than pruned: for each output position, the label the **source** gave
the page now sitting there. The map it needs is `keep` — the source page number at each output
position, repeats included — which `selectPages` already has and which is strictly more than the
kept-page set every other carry uses. That difference is the whole reason this key waited.

**The label follows the PAGE, not the position.** A reorder does not renumber the document. This is
`outlinecarry.go`'s own ruling one key over — *"the surviving order is the SOURCE's, not the new page
order… re-sorting is refused because the outline is an authored HIERARCHY"* — and a page label is
authored the same way: `SetPageLabels` is a user saying where the body starts. Renumbering on a
reorder would be the operation inventing a claim the user never made. It follows that a reordered
output may carry labels that do not ascend, and that a page named twice carries its label twice. Both
are the source's own answer for those pages, which is the only thing this carry is entitled to say.

**A page the source never labelled gets an explicit empty entry**, not silence. Silence makes it
inherit whichever range precedes it in the OUTPUT — a label this document has never made, which is
the failure mode this key has and the outline does not: *a page that says it is page 7 while it is
page 3 is a wrong statement, not an absent one.*

**There is no range ceiling.** A permutation shatters a contiguous range into up to one entry per
page, and the obvious guard — drop the key past N ranges — is refused because the document it
produces is strictly worse: 200 correct labels beats none. The cost that would justify it is not
there. Measured, 200 pages fully reversed into 200 ranges, against the identical document with the
key deleted: **+6,582 bytes**, and no time cost whose sign is stable — over nine runs the labelled
`Collect` was the faster of the two six times and the slower three, a spread wider than any
difference between them. `TestTheWorstCasePageLabelCarryIsAffordable`
is that measurement; consecutive runs are collapsed into one range, so an order-preserving extract of
a two-range document still comes out as one or two.

### 2. A key with two forms is SPLIT BY FORM, and ONE door decides which

`/OpenAction` is an action dictionary **or** a destination (ISO 32000-1 §12.3.2). A destination
positions the view and runs nothing; an action runs, and may be `/JavaScript`, `/Launch`,
`/SubmitForm` or `/GoToR`, and may chain to one through `/Next`.

- A **destination** is carried when it still reaches a kept page, through
  `destReachesAKeptPage` — the same predicate the outline and `pruneNames` share, so the three cannot
  disagree about a destination.
- A **`/S /GoTo`** action is read for its `/D` and rewritten as a plain destination. Chain and all,
  the dictionary is dropped.
- Every other action is dropped outright.

That is `outlinecarry.go`'s ruling again, at the door where the stakes are higher: an outline item
runs when a user clicks it, and an open action runs when the file opens. Carrying the dictionary
would re-admit an auto-run hook the subset dropped for free.

**The discriminator is `/S`, not `/Type /Action`.** `/Type` is optional on an action dictionary and
`/S` is required, so keying on `/Type` reads every `/Type`-less JavaScript action as a destination —
wrong in the one direction that matters. A named destination resolves to `<< /D […] >>`, so
"dereferences to a Dict" cannot tell the two apart on its own.

### 3. `Scan` was making the same mistake, and it is the same door

`Scan` reported the **presence** of `/OpenAction` as *"Runs an action automatically when the document
opens"* at high severity. That is true of an action dictionary and false of a destination — a
high-severity finding about a document that runs nothing. It stayed invisible because a subset
dropped the key outright, so the false claim was only ever made about a source document; carrying the
destination form would have started making it about 220 of every 295 extracts.

So the split lives in `openActionForm` and both readers call it (ADR-009). `StripActive` is a
**named exemption**: it goes on deleting the key whole, including the destination form. The two doors
answer different questions — `Scan` says what is HERE, and mislabelling a view as a hook is a false
statement about somebody's document, while `StripActive` removes what could run, and over-removing an
opening view costs a reader one scroll. Reducing a `/GoTo` to its destination there would be the
symmetric answer; it is not taken, because it turns a delete into a rewrite at the one door whose
whole value is that it only ever takes things away.

## Consequences

- Both keys are re-added **on top of** `catalogAllowlist`, never into it, and both behind the `carry`
  gate — so redaction and every subset feeding a composition still drop them. The gate is right for
  `/PageLabels` (a composition keeps only the first part's catalog, which would run part one's
  labelling on over pages it never described) and is **consistency rather than a found leak** for
  `/OpenAction`: a destination names a page and a view, never content, so nothing of what a redaction
  destroyed travels in it.
- `TestASubsetLeavesExactlyTheCatalogItLeftBefore` gains two declared divergences, six in all, and
  goes on grading every other key — `/OutputIntents` included — against the old implementation.
- Scanning a document whose `/OpenAction` is a destination no longer reports an auto-run hook. A
  user who had learned to expect that finding on every such file will stop seeing it; the control in
  `TestScanCallsAnOpenActionAnActionOnlyWhenItIsOne` is what keeps the fix from being "report
  nothing".
- **Still not addressed:** `api.MergeRaw` keeps only the first document's catalog, so neither key
  survives a merge from any document but the first — `/pending 559`'s half of ADR-043's residue,
  untouched here. And a dropped open action or label set is still dropped **silently**: the
  page-operation route has no notice channel (`/pending 574`), and these are two more of the
  sentences it owes.
