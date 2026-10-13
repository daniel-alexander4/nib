# ADR-136 — an associated file only /AF reaches is refused at the write door

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-129 (one door writes), ADR-032 (refuse by name rather than lose silently), ADR-072 (a refusal
about a document is a 422 naming its cause).
**Context:** /pending 655. pdfcpu's writer (v0.13.0) writes the catalog and each page whole, then only what a
fixed list of their keys reaches; `/AF` is on neither list, so the key survives and the file specification and
its stream do not — the key dangles. Measured: a file also listed in `/Names /EmbeddedFiles` (as PDF/A-3
requires), or named by a page annotation's `/FS`, or on a structure element, is carried. So every nib rewrite of
a document whose associated file is reached ONLY through a catalog or page `/AF` deleted that file and left the
document structurally broken — the source spreadsheet behind an invoice, gone on the first save.
**Applies:** `internal/pdfread` (`assocfile.go`, `Write`), every caller of the write door; `httpError`.

## Decision

`pdfread.Write` refuses, with `ErrAssociatedFileLost`, a context read from a document whose catalog or a page
`/AF` names a file none of those roads reaches. It is judged on the document AS READ, by a mark set in
`validated`. The server answers 422 with cause `associated-file`.

## Refused

Carrying the file by entering it in `/EmbeddedFiles` at the write. The write cannot tell "the operation removed
the files" from "nothing ever listed this one", so it would put the file back after *Remove attachments*, and
into extracts and redactions that drop that tree on purpose.

## Consequences

- Such a document cannot be changed in nib; the refusal says why. Nothing is lost silently.
- The read-time pass costs 0.37 / 1.0 / 10.5 ms at 1,000 / 10,000 / 50,000 pages.
- Declared: the mark does not follow objects into another context — the SOURCE of a merge and pages cut or
  imposed into a new document still lose such a file. A document whose files were all listed is written as
  before, including a dangling `/AF` after its tree is removed or a subset taken.
- Handlers that do not pass the error text through `httpError` keep their own status.
- Other catalog and page keys off the writer's lists are unmeasured (ADR-129's record saw `/PieceInfo` go).
