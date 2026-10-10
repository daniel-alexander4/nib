# ADR-120 — tags are removed only when asked, and whole

**Status:** accepted
**Date:** 2026-10-09
**Context:** /pending 854 item 3. The commit writer refuses a document that has a structure tree
(`errCommitTagged`) and a page whose content carries ids with no tree (`errCommitMarked`), so a document that
arrived badly tagged could be corrected element by element and never tagged again. `internal/pdfops/tagremove.go`,
`internal/tagwrite`, `POST /api/tags/remove`, `nib tag remove`, *Remove all tags…* in Review Structure Tree.
**Applies:** every operation that takes a structure tree away on a person's say-so.

## Decision

**1. Removing a tree is its own operation, asked for by name.** `pdfops.RemoveStructure`, reached only through
`tagwrite.Remove` — the door that refuses a signed document (`TestEveryStructureWriteReachesTheSignedDocumentDoor`
now polices three names). Proposing never removes: the commit writer still refuses a tagged document, and its
sentence names the way out. In the window it is a button with a confirmation (ADR-106: nothing destructive is
automatic), and one undo brings the tree back.

**2. It is whole.** Every marked-content sequence that carries an MCID loses its opener and its `EMC` — in page
content and in every form XObject a page draws, each form once however often it is drawn. `/StructTreeRoot` and
`/MarkInfo` leave the catalog; `/StructParents` and `/StructParent` leave every page, annotation and form XObject.
A partial removal — the tree gone and ids left — is exactly the state the commit writer refuses, so it would leave
a document that reads as neither tagged nor taggable.

**3. An artifact stays an artifact.** An `/Artifact` sequence carries no id and names no tree. What a producer
or a reviewer declared decoration is still decoration, and a later commit keeps it.

**4. A document with no tree is `ErrTagsStale` (409)**, the answer an edit gives: the person was shown a tree.

## Consequences

- The operation's declared tag fate is `dropped`, driven and measured like every other
  (`TestEveryDeclaredFateIsTheMEASUREDFate`).
- **Declared gap:** marked content inside an ANNOTATION's appearance stream is not walked — the run reader walks
  what a page draws — so an id there survives. Nothing nib writes puts one there.
- `/Lang`, the title and metadata stay; a PDF/UA identification goes, as with any change (ADR-032).
- Tagged again, nib's own Markdown document reads the same tree as a never-tagged copy and fails no ua1 clause
  that copy does not (veraPDF).
- **Covered by** `TestRemovingTheTreeLeavesAnUntaggedDocumentThatDrawsTheSame` (a page's own marks, marks in a
  form, a form drawn twice, named property lists, an unclosed sequence, a 2-up, LibreOffice's table and figure,
  tagged form fields), `TestRemovingTagsTakesOnlyTheBrackets`,
  `TestATaggedDocumentCanBeTaggedAgainOnceItsTagsAreRemoved`,
  `TestTheRemoveRouteTakesTheTreeAwayAndOneUndoBringsItBack`,
  `TestTheRemoveRouteRefusesASignedDocumentAtTheDoor`, `TestTagRemoveLeavesADocumentThatCanBeTaggedAgain`,
  and `test/jsdom/tagedit.test.mjs`.
