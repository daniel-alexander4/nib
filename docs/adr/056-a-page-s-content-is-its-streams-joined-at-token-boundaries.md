# ADR-056 — a page's content is its streams joined at token boundaries

**Status:** accepted. `PLAN-text-reflow.md` P05.S01.

## Context

A page's `/Contents` may be an array of streams, and ISO 32000-1 §7.8.2 lets the division fall on any token
boundary. pdfcpu's `XRefTable.PageContent` (model/xreftable.go:1882, v0.13.0) appends the decoded streams with
nothing between them, so two legal divisions change what the page says:

- a first stream ending `(A) Tj` and a second beginning `ET` read as the single unknown operator `TjET` — the text
  is not shown and the text object never closes;
- a first stream ending inside a `%` comment comments out the second's first line.

Seventeen nib readers called it, including the positioned-run reader reflow and the autotagger stand on, and
seven writers — `setPageContent`'s six call sites and `wrapPageToBox` — write the join back as ONE stream — making a misreading the document. Measured
over the real-producer corpus (36 files, 9 producers) and veraPDF's PDF/UA-1 corpus (297 files): 69 array pages,
77 joins, **0** in either fused shape. Spec-legal and unseen, which is why nothing had noticed.

## Decision

1. **`pdfread.PageContent` is the one door** to a page's decoded content. A single stream is pdfcpu's call
   unchanged; each stream of an array is decoded through the same call, and a `\n` is inserted between two only
   where a regular byte (or the empty name `/`) meets a regular byte or the earlier ends inside a comment. Everywhere else its output is
   byte-identical to pdfcpu's — so no measured document reads differently through it.
2. **Two named exemptions**, each marked `//pagecontent:exempt <name>` at its site and declared in the guard:
   `ContentDigest` (its coverage is a format — ADR-013 — so changing its join is a `ContentDigestVersion` bump,
   `/pending 718`), and the accessibility checker's content reader (it reads as veraPDF does — ADR-052 — and how
   veraPDF joins is unmeasured, `/pending 719`).
3. `TestEveryPageContentReadRoutesThroughTheDoor` is an AST census over the module: any other call to a method
   named `PageContent` is a finding, a marked exemption not declared for its file is a finding, and a declared
   exemption that matches nothing is a finding.

## Consequences

- A divided page reads and is rewritten as it means; `TestADividedPageReadsItsText` fails on the old join.
- The digest and the checker still read pdfcpu's join, so on a fused page nib's readers and nib's digest see
  different token streams. Declared; both are filed.
- A split INSIDE a token (a string, a dictionary, an inline image) is forbidden by the spec and is not repaired.
- The census sees method calls named `PageContent`; a function value taken from one and called elsewhere would pass.
