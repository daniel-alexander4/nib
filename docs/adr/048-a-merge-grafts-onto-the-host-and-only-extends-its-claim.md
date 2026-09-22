# ADR-048 — a merge grafts onto the host, and only extends a claim the host already makes

**Status:** accepted · v1.138.12 · amends ADR-031's note on `Append`/`Combine`

## Context

ADR-031 recorded `Append` of a tagged and an untagged document as `partial`: `api.MergeRaw` keeps the
first document's catalog, so its tree survives and the appended pages are undescribed under its claim.
That note was about an untagged SECOND document, and the census only ever drove that case.

**Two tagged documents were worse, and nothing saw it.** A page's `/StructParents` is an integer, so
`MergeXRefTables` — which renumbers every object reference of the second document — never touches it.
`Append(tagged, tagged)` wrote both pages with `/StructParents 0`, so page 2, document B's words,
resolved to document A's element: a page asserting another document's text (probed 2026-09-21, fate
`partial`, one completeness defect). `Combine`, and so the Combine card and `nib combine`, did the same.

The graft that would fix it cannot sit behind `MergeRaw`: after `MergeXRefTables` the second document's
tree is still in the merged context, renumbered, but its catalog has been freed, so the write leaves the
tree out. There is no place between the merge and the write to stand.

## Decision

**nib owns the merge loop** (`internal/pdfops/merge.go`, `mergeDocs`), doing what `MergeRaw` did in
the same order and configuration, and adding a graft between the merge and the write.

**A merge only EXTENDS a claim the host already makes** (Dan, 2026-09-21, via /discuss; the host made
precise by the pre-slice deepdive). For `Append` and `Combine` the host is the first document — the
order the user chose.

- **Host tagged, later document tagged:** the later document's claims are offset past the host's keys in
  its own context before the merge, and after it its root's kids join the host root (`/P` repointed), its
  `/ParentTree` rows join the host's, and its RoleMap and ClassMap names join the host's. A differing
  catalog `/Lang` is stamped on each grafted top-level element, or the second document would be read in
  the first one's language. Its `/IDTree` is not carried (the subset carry's reason). The tier is the
  weaker of the two, and a tree partly unrecorded records none.
- **Host untagged:** the later document's claims are stripped. Grafting would make the result claim
  tagging over pages nobody tagged — ADR-031's worst case, a screen reader abandoning its fallbacks.
- **Cannot graft** — a nested `/ParentTree` on either side, or a role or class name the two map
  differently: the later document's claims are stripped, and the result is `partial`.
- **The output is gated** on `carryIsComplete`, as every carry is; an incomplete graft re-merges with the
  later documents stripped.

ADR-031's `partial` stands for a tagged host and an untagged later document; this adds `carried` for
two tagged ones, and removes the wrong reverse link from every merge.

## Consequences

- Two root-level `/Document` elements after merging two documents that each have one. PDF/UA-1 permits
  it; UA-2 would not. Nesting under the host's `/Document` is the alternative, left for a UA-2 reason.
- `splice` (`InsertPDF`, `SplitPage`, `SplitRegions`) still cuts the original with the non-carrying door
  and its first SEGMENT is the host — the wrong host for an insertion at page 1. That is P02.S07b.
- The ceremony's appends are unchanged today: nib's own pages carry no claims. When P02.S09 tags them,
  they graft onto a tagged original and are stripped from an untagged one.
- `withoutUAClaim`'s second full write after every merge is gone: the PDF/UA identification is dropped
  inside the merge's own write (ADR-032).
