# ADR-070 — the patched reader refuses an object stream whose /Type, /N or /First is a reference

**Status:** accepted. /pending 768. **Supersedes in part** ADR-067's consequence that objects a stream
dictionary references "compound per level and are declared, not charged".

## Context

The patched reader (ADR-066) reads an object stream's `/Type`, `/N` and `/First` afresh on every lookup. When one
is a reference it resolves another object per lookup, and where that object sits in an object stream whose own
dictionary does the same, the cost compounds per level — measured at /pending 760 as 3^d reads a lookup, 27 s a
pass at d = 8. ADR-067 declared it rather than charging it, because pdfcpu refuses every such stream and so
`Verify`'s gate kept it out. /pending 712 then decided (Dan, 2026-09-30) that `HasSignatureBlob` stays outside that
gate, which left the shape reachable on the paths that ask it — save, undo, reflow, tag writes, the CLI — on bytes
pdfcpu had not read.

## Decision

The reader refuses the shape itself: an object stream whose raw `/Type`, `/N` or `/First` is an indirect reference
panics `ErrObjStmIndirectKey` before it is resolved (NOTICE.nib divergence 5). Callers already contain the reader's
panics; `signatureBlobPresent` falls back to its bounded byte scan. Chosen over auditing each ungated caller, which a
new caller would silently undo, and over charging the resolution, which would bound the cost without removing it.

## Consequences

- No document nib opens is affected: pdfcpu refuses these streams (measured at /pending 760), so none reaches an open.
- **Verified by review only**, by Dan's choice (2026-09-30): no fixture of the compounding shape was built. The
  library's existing tests, the patch guard and the object-stream fuzz test are green with the change.
- `/Extends` is still resolved per lookup; it names a top-level stream, which cannot sit inside an object stream.
