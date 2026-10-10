# ADR-126 — an inline tag is given a number, not an address

**Status:** accepted
**Date:** 2026-10-09
**Context:** /pending 855 item 3. A structure element may be written inline — a dictionary inside its parent's
`/K` rather than an object of its own. Such an element has no object number, and everything the tree editor
does names an element by its number: an edit's `Element`, a move's or a create's `Parent`, a `/P`, a
`/ParentTree` slot, a header cell in `/Headers`. So an inline element could not be retyped, moved, deleted,
given anything to hold, or named as a header, and neither could anything be moved out of one or deleted from
inside one (ADR-124's fourth refusal; ADR-119's gap G2). The edit bar for one was a row of disabled controls.
`internal/pdfops/structpromote.go` (`promoteInline`, `claimEmptySlot`, `defectsAPromotionUncovers`),
`structedit.go` (`applyStructEdits`), `structtree.go` (`structElem.objNr`), `structcheck.go`;
`/api/tags/edit`, `nib tag edit`, the Review Structure Tree panel in `web/app.js`.
**Applies:** every edit of an existing structure tree.

## Decision

**1. There is no second address for an inline element.** A path (`0/2/1`) or a position would be an identifier
every other edit in the same batch moves, and a second way to name an element that every edit kind, the route,
the CLI and the panel would each have to carry. The one thing done to an inline element is to stop it being
inline.

**2. One edit kind, `promote`, for the whole document.** `StructureEdit{Kind: "promote"}` names no element and
takes no value, parent or header cells — any given is `ErrTagsReview`. Every inline element of the tree becomes
an indirect object: the SAME dictionary, written as a new object, its place in its parent's `/K` taken by the
reference. The order of every `/K` is unchanged, and a `/K` written as its own array object is updated in that
object. It goes through the batch that exists: one route, one signed-refusal door (`tagwrite.Edit`), one undo,
and the consistency check before and after.

**3. Top-down, so `/P` is one line.** `readStructTree` lists a parent before its kids and the elements are
promoted in that order, so an inline element's parent has its number — its own, or the one just given — by the
time the element is reached. An inline element under an inline element needs no second pass.

**4. What a promoted element is told.**

- **`/P`** names its parent — the root's reference for a top-level element — written even where the inline
  dictionary had none: ISO 32000-1 Table 323 requires it.
- **Its element kids' `/P`** name it, where the tree lists the kid under it first (the rule a delete re-homes
  by, ADR-124). Nothing could name the parent before, so whatever such a kid's `/P` said, it was not its parent.
- **Every marked-content id it owns** gets its `/ParentTree` slot **where the slot is null or absent**: the
  page's row for an integer MCID or an MCR, and for an MCR with `/Stm` that stream's row as well (ADR-038).
- **An OBJR kid's annotation** gets its `/ParentTree` entry where it carries a `/StructParent` that has none.

**5. What it leaves.** A slot or an annotation entry that names another element is a defect the tree already
had; it is left exactly as it is, as a delete's `passSlot` leaves one. A page key with no row is given none. A
row is grown to reach an id only up to `promoteMaxSlot` (65,536): the id is the document's own number, and a row
filled with nulls up to two thousand million would be a memory bomb one integer long. An annotation with no
`/StructParent` is given no key — that is authoring the annotation. An annotation key that falls inside a
nested number tree's range, where placing an entry means re-sorting a leaf, stays without one.

**6. A no-op is refused.** A tree with no inline element answers `ErrTagsReview` — "no tag in this document is
written inline" — so a write that changes nothing is never an undo step.

**7. A defect the checker could not name before is not blamed on the promotion.** `checkStructConsistency`
keys a defect by the element's object number, which was 0 for every inline element, and one of its questions —
does the slot name the element that claims the id? — is not asked of an element with no number at all. The
moment the element has a number those defects read as new and would refuse the batch. After a promotion the
batch therefore excuses the defects *about the elements it just numbered* (`defectsAPromotionUncovers`) —
except a null slot under an id the element claims: filling those is the promotion's own job, and one left
empty is a defect it added.

**8. The editor replaces the dead end.** When the selected tag is inline the bar shows one line — "This tag is
written inline in the file, so it cannot be changed until it is given a number of its own." — and one button,
**Make inline tags editable**, which sends the one `promote` edit. After the re-read the tag at the same
POSITION in the tree is selected and focused, and the bar is live: the tag has a new number, the order has not
changed, so position is the identity here. The status says it is one undo step. The reasons *Move into the tag
above*, *Move out one level* and *Add tag* give for an inline neighbour end by naming the button.

## What changed on the way

- **The design said the reading "adds no defect"; the checker said it added several.** Decision 7. The first
  run of the promotion on a tree whose inline element claimed an id a slot gave to another element was refused
  by the after-check — a defect that was there all along under a name the check could not form.
- **The design said veraPDF's verdict would not change. It improves.** veraPDF finds a marked-content id's owner
  through the `/ParentTree`, and no slot can name an inline element — so it read an inline element's content as
  untagged and its tag as owning nothing. Measured on veraPDF 1.30.2, on a tree whose inline elements own
  content: 7.1 t3 and 7.2 t4, t8 and t9 fail before the promotion and pass after, and no clause is added. On a
  tree whose inline elements only group other elements, the failed clauses are identical.
- **An MCR with a `/Stm` claims two slots.** The checker asks the PAGE's row of every MCR, with or without a
  `/Stm`, and a delete hands on both rows; the promotion fills both where empty, or the after-check would
  report the page row's null as a defect the promotion left.

## Consequences

- **Closed:** ADR-124's refusal of a delete inside an inline element no longer applies once a document is
  promoted, and that refusal now says so; ADR-119's G2 — a `TH` written inline could not be named as a header.
- **Declared, not done:** an annotation with no `/StructParent` is not given one; an id past `promoteMaxSlot`
  beyond the end of its row gets no slot; one inline tag cannot be promoted alone.
- **No producer in the local corpus writes an inline element.** Measured 2026-10-09: 0 of the 22 tagged
  documents under `~/nib/producers` (36 files) and 0 of the 294 tagged documents of veraPDF's corpus. The edit
  is proved on hand-built trees only; the form is legal (Table 323's `/K` admits a dictionary) and
  `readStructTree` has always read it.
- **Covered by** `internal/pdfops/structpromote_test.go` — inline at depth, inline under inline, integer, MCR,
  `/Stm` and OBJR kids, a flat and a nested `/ParentTree` with an indirect row, every form of `/K`, each `/P`,
  each slot, the slots left, the bound, the refusals, the edits that then work (delete, retype, move, headers),
  and `TestAPromotionAddsNoUA1Failure`; `TestTheEditRoutePromotesInlineTagsAndOneUndoTakesItBack`;
  `TestTagEditPromotesInlineTagsAndEditsTheRoleMap`; `TestDecodingReadsAPromoteAndARoleMapEdit`;
  `test/jsdom/tagpromote.test.mjs`; and `test/ui/tagpromote.test.mjs` — an inline tag reached, numbered,
  retyped and undone twice by keyboard alone.
