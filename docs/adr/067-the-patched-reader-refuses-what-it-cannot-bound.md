# ADR-067 — the patched object-stream reader refuses what it cannot bound, and is fuzzed against the original

**Status:** accepted. /pending 759, 760. **Supersedes ADR-066 in part:** its statement that the patch has exactly
one declared divergence from digitorus/pdf v0.1.2. The rest of ADR-066 stands.

## Context

ADR-066 carried a lazy object-stream cache in `third_party/digitorus-pdf/read.go`. Measured afterwards: header ids
naming one offset re-read that member per lookup (100 ids × a 1 MB member, 7.5 s a pass; 400, 44 s), and offsets into
one nested array are quadratic (8,000, 30 s) — shapes pdfcpu reads, so Verify's readability gate (ADR-041) passes them,
and the lookup-cost model (objstm.go) cannot charge a member's extent without paying for it. And the patch was proved
faithful only on enumerated cases.

## Decision

- **The reader bounds itself:** member reads may consume at most 4× what the stream has decoded plus 4 KB a stream and
  64 B a lookup, and one stream decodes at most 64 MiB; past either it panics `ErrObjStmTooCostly`, which the sweep maps
  to `errLookupCostCeiling`, so the verdict is Invalid / could-not-check with `Unchecked: lookup-cost` (named in the CLI
  and on the page). **This is the second declared divergence** from v0.1.2, beside the backward `/First`.
- **The patch is fuzzed against the original:** v0.1.2's reader is kept unchanged under `internal/sign/testdata/`
  (never shipped; a test holds it byte-identical to the module cache) and `FuzzThePatchedObjectStreamReaderMatchesTheOriginal`
  compares values and panics; the two declared divergences are the only ones allowed. A 5-minute run (464,211 execs)
  found one real divergence — a truncation panic reporting a different offset — fixed, and kept as a seed.

## Consequences

- Upstream still loops forever on `endobj` inside an array (both readers); pdfcpu's gate covers Verify, not paths that
  skip it. Objects a stream dictionary references compound per level and are declared, not charged.
