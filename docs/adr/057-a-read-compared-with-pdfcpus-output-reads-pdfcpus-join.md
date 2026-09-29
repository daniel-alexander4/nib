# ADR-057 — a read compared with pdfcpu's own output reads pdfcpu's join

**Status:** accepted. Supersedes ADR-056 in part (its list of exemptions). `PLAN-text-reflow.md` P05 phase close.

## Context

ADR-056 routed every page-content read through `pdfread.PageContent`, which joins a `/Contents` array at token
boundaries, and named two exemptions. Two more readers do not compare a page with itself but with bytes **pdfcpu
wrote**: `api.NUp` puts each source page into a form built from pdfcpu's own bare join (`nup.go:328`), and the note
carry (`annotcarry.go`) and the tag carry (`tagcarry.go`) identify a placement by comparing that form with the source
page. Routed through the door, a divided page's content gained a separator the form lacks, the comparison failed, and
the note or the tag tree was dropped — found by the phase-close review, reproduced by
`TestANUpCarriesTheNoteOfADividedPage` (0 notes) and `TestANUpCarriesTheTagsOfADividedPage` (2 elements → 0).

## Decision

A read whose bytes are **compared with output pdfcpu produced from its own join** reads pdfcpu's join, as a named
exemption (`annotcarry-nup`, `tagcarry-nup`) declared in `TestEveryPageContentReadRoutesThroughTheDoor`. ADR-056's rule
and its two exemptions otherwise stand.

## Consequences

- The carries follow whatever pdfcpu writes: when pdfcpu's `NUp` output itself fuses a divided page (`/pending 728`),
  the carry matches that fused form, and closing 728 means revisiting both exemptions together.
- Four exemptions now, each excusing exactly one call.
