# ADR-084 — pdfcpu's page operations are handed separated contents, and the n-up carries read the door

**Status:** accepted. `/pending 728` (2026-10-03). Supersedes ADR-057; extends ADR-056 under ADR-009.

## Context

ADR-056 made `pdfread.PageContent` the one door to a page's decoded content: a `/Contents` array is joined at token
boundaries, where pdfcpu's own `PageContent` appends the streams with nothing between them — so a page divided
`(A) Tj` | `ET` reads as the unknown operator `TjET`, and a first stream ending in a `%` comment swallows the second's
first line. The door fixed nib's readers. It could not fix pdfcpu's **writers**: `NUpFromPDF` (nup.go:328), `Resize`
(resize.go:276) and `CutPage` (cut.go:175/263/353) read the page through pdfcpu's join and write what they read into
the output. Measured on `testpdf.SplitContents`: pdftotext reads the divided input's text and **none** of the
outputs of n-up, normalise-page-sizes, the grid split or the region split — each held `TjET`.

ADR-057 had already bent the door around this: the n-up's note and tag carries compare the n-up's forms with the
source page, the forms held pdfcpu's fused join, so the carries were exempted to read that join too.

## Decision

1. **`pdfread.SeparateContents` is the one door before a pdfcpu page operation.** Wherever the door would put a
   `\n` between two elements of a `/Contents` array, it puts a stream holding only `\n` into the page's array. No
   stream is edited (a shared or repeated stream is safe), and a page needing no separator — every page measured at
   ADR-056 — is left exactly as it was. pdfcpu's bare join of the result is byte-identical to the door's join of the
   original, so pdfcpu's output now means what the page meant.
2. **Every call to a join writer goes through it.** `nUpSeparated` replaces `api.NUp` step for step (the same
   `ReadAndValidate`, `NUpFromPDF`, `api.Write`) with the door before the composition; `cutSeparated` is the one
   `CutPage` caller; `NormalizePageSizes` separates before `Resize`. `TestEveryPdfcpuJoinWriterIsHandedSeparatedContents`
   is the census: a `pdfcpu` join writer with no door earlier in its function, or any `api` entry point to one (it
   reads the document itself, so it cannot be handed a separated context), is a finding.
3. **ADR-057's two exemptions are withdrawn.** The n-up's forms now hold the door's join, so the note and tag carries
   read the source through `pdfread.PageContent` like every other reader.
4. **The census sees key reads, not only calls.** A reader that walks a page's streams itself never calls a method
   named `PageContent`, so `resourceprune.go` decoded every stream of a page unseen. A read of the key `/Contents`
   (an index or `.Find`, not an assignment) now needs a marker: `//pagecontent:exempt <declared name>` (the resource
   pruner, whose per-stream reading is the door's meaning; the stamp-prefix fix, which edits one stream), or
   `//pagecontent:key <reason>` where the key read is not a page's content (annotation text, a signature's blob).
5. **The door decodes each distinct stream once per page.** An array naming one stream N times decoded it N times;
   a 1 KB file naming an empty stream 100,000 times cost 5.5 s and 3.9 GB with no output, which `MaxPageContentBytes`
   cannot see because nothing is produced.

## Consequences

- A divided page survives n-up, resize and both splits; `TestPdfcpusPageOperationsKeepADividedPagesText`.
- Exemptions from the door are now three: `ContentDigest` (ADR-013, `/pending 718`), `resourceprune-names` and
  `stamp-prefix`; the checker reads `PageContentAsPdfcpu` (`/pending 719`).
- pdfcpu's own walk still decodes a repeated element once per mention when nib hands it a page: the door bounds
  nib's reads, not pdfcpu's.
- A function value taken from a join writer and called elsewhere would pass the census, as ADR-056 says of its own.
