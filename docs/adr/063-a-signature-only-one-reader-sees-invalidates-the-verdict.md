# ADR-063 — a signature only one reader can see makes the verdict `Invalid`, whatever else verified

**Status:** accepted. /pending 749. **Supersedes ADR-062 in part:** its statement that `State` is unchanged by
`Unchecked`, and its consequence that "a hybrid file on which the library still sees SOME signers gets the ordinary
verdict". The rest of ADR-062 stands.

## Context

nib's signature reader (digitorus, and `sign.Revisions`, which sweeps as it does) never follows a trailer's
`/XRefStm`; pdfcpu does. ADR-062 named the gap only when the library found NO signer. Measured on a file whose
classic table lists Alice's signature and whose `/XRefStm` lists Bob's: `valid`, 1 signer, `AddedAfter=appended`,
nothing named — and it would have read the same had Bob's signature failed. A verdict over a population nib did not
read.

## Decision

**`verifyIndexed` keeps pdfcpu's reading and compares populations.** `unseenSignatures` reports a signature-shaped
dictionary (the one shape test, `signatureShaped`, shared with the sweep) with non-empty `/Contents` that pdfcpu
holds and the sweep has no record of; object-stream members are decoded only when their raw bytes could hold a
signature key. When one exists, `withUnchecked` — on the signer path as well as the zero-signer path — sets
`State=Invalid`, `Unchecked` (`hybrid-reference`, or `unread` outside a hybrid file) and `could-not-check`; `Signers`
still lists the ones checked. `Revisions` errors.

**`Invalid`, not a new state:** it is what /pending 733 and 741 chose for this condition, every reader of `State`
(`p2p/l3.go`, CLI exit codes, ceremony gates) fails closed on it, and `Unchecked` carries the why. No transmitted
format changes.

## Consequences

- The gap is named only where it exists: none of the 36 real-producer files moves (the two unsigned hybrids stay
  unsigned, the signed ones valid).
- Cost: 0.23 ms on a 100 MB document whose object stream holds nothing signature-shaped; 222 ms at 40,000 members
  that all pass the prefilter.
- A `/Perms /UR3` signature held only in a hybrid stream beside a real signer reads `Invalid` — ADR-059's `/Perms`
  exemption does not reach it; the safe direction.
