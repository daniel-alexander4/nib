# ADR-064 — a stream is decoded within what is left of its budget, at one door, and page content is bounded where pdfcpu decodes it

**Status:** accepted. /pending 748. Extends ADR-005's 512 MiB figure to decoded page content; follows /pending 721
and 742 (repeat decodes).

## Context

pdfcpu v0.13.0 already stops a single `Decode()` at 512 MiB (`filter.DefaultMaxDecodeBytes`), so the item's
"one stream inflates to gigabytes" was false. What was live: every nib budget was charged AFTER a decode returned,
and nothing capped the SUM — a `/Contents` array or many pages naming one stream N times. Measured on base: a
~100 KB file naming one 100 MiB stream 40 times, `ReadOptimizedOrRefuse` 36.7 s at 9.8 GiB peak; `PageContent`
returning 1.2 GiB for six namings; a form past 512 MiB skipped with no error (a silent false-complete). And the
first decode was pdfcpu's own optimize pass (`optimizeResourceDicts` → `consolidateResourcesWithContent` →
`PageContent`), under the default VALIDATE command — not nib's decode doors.

## Decision

- **One capped decode, `pdfread.DecodeWithin(sd, limit)`** (ADR-009): `DecodeWithLimit` so the cap binds during the
  decode, every filter included; uacheck's `decodedContent`, pdfops' `formWalkBudget.formContent` and uacheck's XMP
  read call it with what is LEFT of their budget. `MaxPageContentBytes` (512 MiB) is the one page-content figure;
  uacheck's `maxContentBytes` is it.
- **The page-content joins are capped** (`PageContent`, and `PageContentAsPdfcpu` for ADR-057's bare join).
- **pdfcpu's pass is estimated before it runs** (`contentcost.go`), and only where it will decode:
  `passDecodesPageContent` restates pdfcpu's own condition in one predicate (pinned to v0.13.0 by a test), and
  `contentMayExceed` opens a decode only when a stream is named more than once — the amplification that costs six
  bytes a naming. A raw length × worst-expansion trigger was built, measured and REFUSED (Dan, 2026-09-29): it
  opened a decode on every document past ~508 KiB of raw page content, +21-62 % on a 200-page, 50 MiB-content
  read, to close a gap that is bounded by the file's size anyway. An affordable estimate leaves each decoded stream on its entry so pdfcpu's pass does not decode
  it again; the writer writes `sd.Raw`, and a stream-by-stream raw-hash comparison over 36 producers shows the
  written bytes unchanged.

## Consequences

- The open path (`LISTPROPERTIES`), every real producer file and any document naming each content stream once
  allocate nothing extra: no stream is decoded by the estimate. A document that repeats a naming is decoded once by
  the estimate and not again by the pass, and holds its decoded page content for the context's life (measured on
  50 MiB of content: +53 MiB retained).
- **Declared gap:** distinct streams that each inflate towards pdfcpu's 512 MiB per-decode cap are not estimated,
  even when their sum passes the budget — six different 100 MiB streams (~600 KB of file) pass. The pass's cost
  there is bounded by pdfcpu's cap times the streams the file carries, i.e. by the file's size times flate's
  ~1032:1. `TestDistinctStreamsAreADeclaredGap` fails if the gap closes, so closing it is a new decision.
- Still bounded only by pdfcpu's per-decode cap: font, CMap, glyph and attachment decodes (many distinct streams, no
  spanning budget), and three `ctx.PageContent` reads — ContentDigest (ADR-013 territory) and the n-up carries.
  Peak heap is ~2× any limit (pdfcpu's buffer doubling).
