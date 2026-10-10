# ADR-127 — the role map is published, and a custom type maps only to a standard one

**Status:** accepted
**Date:** 2026-10-09
**Context:** /pending 855 item 4. A document's `/RoleMap` says what each of its own structure type names means
(`/Heading#201 → /H1`). Nib read it — `structTree.roleMap`, `standardRole` — to show each element's type as
written and as resolved, and neither published the map itself nor let anyone change it. A custom type mapped
to nothing (ua1 7.1 t5) could only be repaired by retyping every element that carried it.
`internal/pdfops/structrolemap.go` (`roleMapView`, `setRoleMapping`), `structview.go` (`RoleMapping`,
`StructureTree.RoleMap`), `structedit.go`; `/api/tags/tree`, `/api/tags/edit`, `nib tag tree`, `nib tag edit`,
the Review Structure Tree panel in `web/app.js`.
**Applies:** every read and every edit of an existing structure tree.

## Decision

**1. The role map is part of the tree's answer.** `ReadStructure`, the tree route and `nib tag tree` carry
`roleMap`: one row per mapping the reader holds — the custom **name**, the type it is mapped **to** as written,
the **standard** type that leads to (`standardRole`; the name itself where the map leads it in a circle, as an
element typed with it reads), and how many **elements** are typed with the name. Sorted by name; an empty list,
never null, for a tree with no map and for an untagged document. One shape for the route and the CLI
(`TestTheCLIsJSONIsTheRoutesShape`).

**2. One edit kind, `rolemap`, one entry at a time.** `StructureEdit{Kind: "rolemap", Role, Value}` — `Role` a
new field, the custom name. A standard type in `Value` sets or replaces the mapping; an empty `Value` removes
it. It names no element. It goes through the batch that exists: one route, one signed-refusal door, one undo.

**3. The arrow always ends on a standard type.** `Value` must be one of the standard structure types. A mapping
onto another custom name is how a chain is written, and a chain is how a loop is written (ua1 7.1 t6); an edit
that can only point at a standard type can write neither. Chains and loops a producer wrote are read as they
always were and are not rewritten — replacing a chained mapping is how a person straightens one.

**4. Six refusals, each `ErrTagsReview` with a sentence.**

- `Role` **empty**.
- `Role` **is a standard type**: *the standard structure types shall not be remapped* (ISO 14289-1 §7.1;
  veraPDF's ua1 7.1 t7, `uacheck`'s `checkStandardTypeNotRemapped`). veraPDF fails this only where an element
  carries the remapped type; the edit refuses it whether or not one does.
- `Value` **not a standard type** (decision 3).
- **Removing a mapping while N elements are still typed with the name**: they would be left with a type
  nothing explains. The sentence says how many, and to change their type first.
- **Removing a mapping that is not there.**
- **Setting the mapping it already has**: nothing would change, and a write that changes nothing would still
  be an undo step.

A `/RoleMap` that is not a dictionary is refused too, as a fact about the document rather than the request.

**5. Written where it lives.** A direct dictionary or one that is its own object is changed in place; the key
is created on the root for a first mapping and removed with the last, since an empty map explains nothing.
Retype stays standard-types-only: a mapping may be added for a name no element uses, and nothing turns an
element INTO a custom type.

**6. Names are written as they are read.** pdfcpu decodes a name's `#xx` escapes when it parses one — a
dictionary key and a name value alike — and encodes them when it writes. `readStructTree` therefore holds
`Heading 1`, an element's `/S` reads `Heading 1`, and the inverse of that reading is to store the key as read;
the file then says `/Heading#201`. Nothing escapes by hand: a second escaping would write `#2320` and the name
would stop matching the elements typed with it.

**7. The editor has a Role map section.** A native `<details>` in the Review Structure Tree panel, closed by
default, last in the panel, whose summary says how many custom types the document maps. One row per mapping:
the name, a picker of the standard types preset to its target, **Change** and **Remove** — Remove disabled,
with the reason in the row, while tags use the name — and a last row to add one (a name field, a picker,
**Add**). A button applies, never a picker's change event. After the re-read focus returns to the row that was
used (to the name field when the row is gone), and the tag that was selected stays selected: its line now
reads the type the map gives it.

## What changed on the way

- **Setting the mapping a name already has was not in the design's list of refusals**; it is refused for the
  reason a promotion of nothing is (ADR-126).
- **The design's clause numbers were not the checker's.** A custom type mapped to nothing is ua1 7.1 **t5**, a
  circular mapping **t6**, a remapped standard type **t7** (`internal/uacheck/rules_structure.go`).
- **A mapped type can fail a clause of the type it now is.** Measured on veraPDF 1.30.2: mapping the unmapped
  type to `Note` cleared 7.1 t5 and added 7.9 t1 (a Note needs an `/ID`); mapping a heading name from `H1` to
  `H2` added 7.4.2 t1 (a skipped heading level). The edit says what a name means; the checker says what that
  meaning then requires.

## Consequences

- **Declared, not done:** a mapping onto another custom type cannot be written; a standard type cannot be
  remapped even where no element carries it; an element cannot be retyped to a custom type; a mapping whose
  value is not a name is not listed (the reader holds none for it) though it can be removed by name.
- **veraPDF, measured** (`TestMappingACustomTypeClearsItsUA1Clause`): a tree with a custom type mapped nowhere
  fails 7.1 t5; with the type mapped to `P` it fails every clause it failed but that one; replacing a sound
  mapping, removing an unused one and adding one for an unused name then change nothing.
- **Producers, measured 2026-10-09:** 19 of the 22 tagged documents under `~/nib/producers` carry a role map
  (Acrobat 8, Designer 2, InDesign 2, LibreOffice 4, pdfLaTeX 1, Word 2), from 2 to 41 mappings.
  `TestARoleMapEditOnEveryProducersRoleMap` maps the first custom name of each to another standard type and
  back: in between only the elements whose type leads through that name read differently, afterwards the tree
  reads as it did, and the consistency defects are the ones the document had.
- **Covered by** `internal/pdfops/structrolemap_test.go` — the published rows, set / replace / remove on a
  direct and an indirect map, a name with a space, the key coming and going, every refusal;
  `TestTheTreeRoutePublishesTheRoleMapAndTheEditRouteChangesIt`;
  `TestTagEditPromotesInlineTagsAndEditsTheRoleMap`; `TestDecodingReadsAPromoteAndARoleMapEdit`;
  `test/jsdom/tagrolemap.test.mjs`; and `test/ui/tagrolemap.test.mjs` — a mapping changed by keyboard alone,
  the heading's line in the tree reading its new type, and undone.
