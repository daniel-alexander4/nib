# ADR-124 — a tag is created empty, and deleted without its content

**Status:** accepted
**Date:** 2026-10-09
**Context:** /pending 855 item 1. The tree editor could change an element and move it among its neighbours, and
could neither add one nor take one away: a tree missing a section, or carrying a wrapper a producer invented,
stayed that way. The only edit that removed an element was *Mark as decoration*, which removes its content
from the tree with it — the opposite of what "this tag should not be here" means.
`internal/pdfops/structcreate.go` (`createElement`, `deleteElement`), `structedit.go` (`placeAmongElements`,
`replaceInParent`, `rootParent`), `structwrite.go` (`newGroupingElement`, `setParentTreeSlot`);
`/api/tags/edit`, `nib tag edit`, the Review Structure Tree panel in `web/app.js`.
**Applies:** every edit of an existing structure tree.

## Decision

**1. Two edit kinds, through the batch that already exists.** `create` and `delete` are `StructureEdit` kinds
beside the eight there were: one request shape, one route, one signed-refusal door (`tagwrite.Edit`), one undo
for a batch, and the consistency check before and after.

**2. A created element owns nothing.** It is the grouping element `addGroupingElement` writes — `/Type
/StructElem`, a standard `/S`, a `/P` naming its parent, an empty `/K` — from the one function that writes that
dictionary (`newGroupingElement`). It gets no `/Pg` and no `/ParentTree` entry. `Value` is its type, `Parent`
its parent (0 or -1: the structure tree root), and `Index` its place among the parent's ELEMENT kids, by the
one placement rule a move also uses (`placeAmongElements`): before the element that holds that place, marked
content between elements not counted, appended when negative or past the end. A create that names an element
is refused: it has none to name.

**3. A delete takes the tag and keeps what it held.** The element's `/K` entries take its place in its parent's
`/K`, in order. Nothing in any content stream changes. What the element owned, its parent owns:

- an **element kid**'s `/P` names the new parent (the root, for a top-level delete);
- every **`/ParentTree` slot that named the element** names the new parent — the page's row for an integer
  MCID or an MCR, the form XObject's row for an MCR with `/Stm` (ADR-038), and the single entry under an
  annotation's `/StructParent` for an OBJR. A slot that names something else was not this element's to hand
  on and is left;
- the element's **`/ID`** goes with it: its `/IDTree` entry is removed (a leaf left empty leaves its parent,
  a tree left empty leaves the root), and the identifier is taken out of every `/Headers` that names it,
  through `withTableAttribute` (ADR-119) — unless another element carries the same identifier.

**4. Two page-inheritance rules, because a kid may take its page from the element being deleted.**
`readStructTree` hands an element's page down to what it holds, so re-homing a kid can silently move its
content to another page.

- **An integer MCID** means "on the owning element's page". It stays an integer only where the new parent
  NAMES the deleted element's page in its own `/Pg` — a page it only reads by inheritance is this reader's
  courtesy, and ISO 32000-1 hands no page from element to element; otherwise it is written as `<< /Type /MCR /Pg … /MCID n >>` naming the page
  it was on. The new parent's own `/Pg` is never changed.
- **An element, MCR or OBJR kid that names no page** is given the deleted element's, in writing, where that is
  not the page the new parent names. An MCR or OBJR is written as a copy, never through the object.

The page compared is the one the new parent names itself. A deleted element with no page at all — none named,
none inherited — has nothing to hand on, and nothing is written.

**5. Four refusals, each `ErrTagsReview` with a sentence naming the element and what to do instead.**

- A **top-level element that holds content itself** (an MCID, MCR or OBJR kid): the root's `/K` may hold only
  structure elements (ISO 32000-1 Table 322), so its content has nowhere to go.
- The element whose going **would leave the tree with no element** (ADR-031, as the artifact edit).
- An element **inside one written inline**: the new owner has no object number, so no `/ParentTree` slot and
  no `/P` can name it.
- An element **itself written inline**, which no edit can name.

**6. A move can name the root: `Parent` -1.** A move's 0 has always meant "the parent it has", and the editor
sends it with every reorder, so the root needed a name of its own. A create accepts 0 as well as -1, having no
current parent to keep.

**7. The editor has four controls for this, all buttons.** *Add tag* (a type picker and a button) puts an empty
tag directly after the selected element, or last at the top of the tree with none selected; the new tag is
then selected and focused. *Delete this tag — keep its content* sits beside *Mark as decoration* with a line
that says what each does. *Move into the tag above* and *Move out one level* are the existing move, naming a
parent: the last kid of the previous sibling, and the next sibling after the parent. A move that cannot be
made is disabled and the line under it says why.

## What changed on the way

- **`setParentTreeSingle` could not re-point an annotation's entry**: it creates an entry and refuses a key
  that has one. A delete re-points through `passSingle`, which writes only where the entry names the deleted
  element.
- **`setParentTreeSlot` replaced an indirect row with a direct array.** Measured on a fixture whose page row is
  its own object: the reader's answer was right and the row's object was left behind unreferenced. It now
  writes the row where it lives, as `clearParentTreeSlot` did.
- **A move could not reach the root**, so *Move out one level* from under a top-level element had no edit to
  send: decision 6.

## Consequences

- **Declared, not done:** a tag's title cannot be edited; an element written inline still cannot be edited,
  deleted, or be the parent of anything (/pending 855 item 3); a region of a page the tagger missed still
  cannot be tagged (/pending 855 item 2); the role map cannot be edited.
- **A delete does not judge the tree it leaves.** List items handed to a Document, or a cell to a table, are
  the reviewer's to see; the checker says what a validator would.
- **A tree that already disagreed with itself about the deleted element's content refuses the delete**: the
  claim would become the parent's, which the consistency check reads as a defect the edit added.
- **veraPDF, measured on LibreOffice's own document:** an empty element created, a wrapper created and filled,
  that wrapper deleted, the Document deleted, and a paragraph deleted each fail exactly the ua1 clauses the
  producer's document fails (`TestCreateAndDeleteAddNoUA1Failure`).
- **Covered by** `internal/pdfops/structcreate_test.go` — placement, the root, each refusal, re-homing, both
  page rules, every kind of kid, the `/ParentTree` owners read directly, the identifier, and
  `TestADeleteKeepsEveryWordAndEveryContentStream`, which holds every element's text and every content
  stream's bytes across both edits; `TestTheEditRouteCreatesATagFillsItAndDeletesItAgain`;
  `TestTagEditCreatesATagMovesIntoItAndDeletesIt`; `test/jsdom/tagedit.test.mjs`; and
  `test/ui/tagcreate.test.mjs`, the whole round trip by keyboard alone.
