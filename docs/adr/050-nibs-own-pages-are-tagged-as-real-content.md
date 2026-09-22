# ADR-050 — nib's own pages are tagged as real content, exactly, before any signature

**Status:** accepted · v1.138.15

## Context

Co-signing appends nib's trust-explainer readme to the user's document; a ceremony also appends a
ceremony page and one or more signature pages (`internal/p2p`). All were drawn with no structure, so a
tagged document came out of preparation still claiming tagging over pages nobody tagged — measured
before this change: a tagged Markdown conversion through `PrepareCeremonyDocument` kept its claim at tier
`Exact` with **35 text runs outside any element at two signers and 36 at eight**. The readme is the page a
screen-reader user signing a document most needs, and it was the one they could not reach.

## Decision

**Nib's own pages are tagged as REAL content** (`PLAN-ua-coverage.md` P02.S09, Dan via /discuss 2026-09-21)
— never as artifacts, which would hide the trust explanation from assistive technology on purpose.

- **The roles are exact, not inferred.** nib composes every line of these pages, so it knows each line's
  role when it draws it: the readme's title is a level-1 heading and each paragraph's wrapped lines one
  body element; each fact on the ceremony page is its own paragraph, with the convener's name kept with
  the fingerprint it reads; a signature page's one line is its heading. They go through
  `pdfops.TagAuthoredPages`, which shares `tagMarkdown`'s tail (ADR-009) and records the `Exact` tier —
  the autotagger would record `Inferred`, and the merge graft keeps the weaker tier (ADR-048), so it would
  have downgraded an `Exact` document.
- **Tagged after the content-language declaration**, which skips a page that is already marked: tagging
  first drops the `/Lang` with no error.
- **Before any signature**, where preparation already runs (`PrepareDocument` refuses a signed document).
- `Append` then grafts these trees onto a tagged document and strips them from an untagged one (ADR-048),
  so nib's pages never make a document nobody tagged claim tagging.

## Consequences

- A ceremony's `DocHash` for documents prepared from this version on differs from one prepared before it.
  Nothing re-renders these pages to compare: every `DocumentHash` recomputation reads bytes already in hand,
  so stored records still match their documents.
- **Declared gap: the signature widgets added at signing are untagged** (PDF/UA 7.18.1). Tagging one would
  rewrite `/StructTreeRoot` and `/ParentTree` inside the signing revision; whether earlier signatures'
  validators report that as a disallowed change is unmeasured (`/pending 576`), so it is unattempted rather
  than impossible.
- Unmeasured: a host whose headings use `/H` rather than `/H1`–`/H6` would receive numbered headings from
  nib's pages, a mix PDF/UA 7.4.4 forbids. Every producer measured in this repo writes numbered headings.
