# ADR-131 — a merge may not reveal what a merged document hid

**Status:** accepted
**Date:** 2026-10-11
**Extends:** ADR-043 (a page selection may not reveal what the source hid) from a selection to a merge. ADR-048
(nib owns the merge loop). ADR-009 (one door).
**Context:** /pending 559. A merge keeps the first document's catalog and frees each later one's. A later
document's pages come across whole — the `/OC /L0 BDC … EMC` bracket in the content, the group named in the
page's `/Resources /Properties` — and the one thing that said the group was switched off, its own catalog's
`/OCProperties /D /OFF`, did not. ADR-043 closed this for a selection and named the merge as still open.

Measured 2026-10-11 with ADR-043's own fixture (a 300×200 pt black box inside a group listed in `/OFF`), the
page rasterised at 40 dpi:

| renderer | the document | `Append(host, it)` | `InsertPDF(host, it)` | `Append(it, it)`, page 4 |
|---|---|---|---|---|
| Ghostscript 10.02.1 | 86 dark px | **18,902** | **18,902** | **18,902** |
| poppler `pdftoppm` | 6 dark px | **18,598** | **18,598** | **18,598** |

The last column is a host that has `/OCProperties` of its own: its layer stayed hidden and the second document's
was drawn, because the second's group was in nobody's list. After this decision every cell reads 86 and 6.

**The worst door was not one the entry named.** `RedactPages` assembles its raster pages and its runs of
untouched pages through `pdfread.MergeRaw`, the repo's other merge loop. ADR-043 keeps each run's
`/OCProperties`; that loop then kept only the first segment's. With page 1 redacted the first segment is the
raster, which has none, so a layer hidden on page 2 was drawn in the redacted document — found by reading the
second caller of `MergeXRefTables`, then driven (`TestARedactionKeepsALaterRunsLayerHidden` was red before the
fix).
**Applies:** every loop that calls `pdfcpu.MergeXRefTables` — today `pdfops.mergeOnce` and `pdfread.mergeRawOnce`.

## Decision

**1. A merge never shows content a merged document hid.** Each later document's optional-content properties
are joined to the host's (`pdfread.UnionOptionalContent`), in the same context and the same write as the merge.

**2. It is a union, not a refusal and not a warning.** A refusal would stop a user appending any layered
drawing; a warning asks them to judge a reveal they cannot see. The union is exact, because the obstacle the
entry feared is gone: `MergeXRefTables` renumbers the source's objects IN PLACE, so the source's dictionary,
held from before the merge (`SourceOptionalContent`), already names its groups by their numbers in the merged
document — the way the structure graft reaches the source's tree (ADR-048).

**3. Each merged-in group keeps the state its OWN default configuration gave it, under the host's.**

- Where the host has no `/OCProperties`, the source's is adopted whole.
- Otherwise the source's `/OCGs` are appended to the host's, and in the host's `/D` **and in every alternate
  configuration in the host's `/Configs`**: a group the source's `/D` had off is listed in `/OFF`; one it had
  on is listed in `/ON` wherever that configuration's `/BaseState` is not `/ON`. "Off" is read as a reader
  reads it — in the source's `/OFF`, or under a source `/BaseState /OFF` and not in its `/ON`.
- The source `/D`'s `/Order`, `/Locked`, `/AS` and `/RBGroups` are appended to the host `/D`'s. Each names
  only the source's own groups.

The `/ON` half is not decoration: the base state is the host's and applies to every group no list names, so a
host that says "everything off unless listed" would otherwise HIDE a later document's visible layers — the
same defect in the other direction.

**4. The host's own state is never touched.** Nothing is removed from a host list, no host group changes list,
and the host's `/BaseState`, `/Name`, `/Intent` and `/ListMode` stand.

**5. One function, called from both loops, at the same point** — after `MergeXRefTables`, with the source's
dictionary read before it. It lives in `internal/pdfread` because `pdfops` imports `pdfread` and both loops
must reach it. It is not gated on the structure graft: a hidden layer is hidden whether or not anybody tagged
the document.

## Declared gaps

- **The source's alternate configurations (`/Configs`) are not merged.** An alternate configuration is a
  named whole-document state, and two documents' lists of them have no pairing. The source's groups are still
  hidden in each of the HOST's alternates (decision 3), so no configuration of the merged document shows what
  the source's default hid.
- **A group hidden only by an alternate configuration of the source** is shown, as it was shown under the
  source's own default.
- **Groups are not pruned or de-duplicated.** The same file merged twice contributes its groups twice, under
  the same names; a splice that then drops a page leaves a group that toggles nothing — ADR-043's dead label.
- **n-up does not merge**, so it is not reached by this; what an n-up sheet does to a layer is its own question.

## Consequences

- `Append`, `Combine`, `InsertPDF`, `SplitPage`, `SplitRegions`, `RedactPages` and `ImagesToPDF` all pass
  through the union. After a splice the page selection carries the merged dictionary under ADR-043's
  reachability guard, unchanged.
- A third merge loop must call the same two functions. Nothing but review holds that today; the census that
  would is a guard on `MergeXRefTables`' callers.
- Tests: `TestAMergedInHiddenLayerStaysHidden`, `TestAMergedInLayerKeepsItsOwnStateUnderTheHostsConfiguration`,
  `TestARedactionKeepsALaterRunsLayerHidden` (`internal/pdfops/optionalmerge_test.go`).
