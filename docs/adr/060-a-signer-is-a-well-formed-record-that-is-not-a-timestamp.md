# ADR-060 — a signer is a well-formed record that is not a timestamp, and `State` is theirs

**Status:** accepted. Completes ADR-058 ("who signed") and ADR-059 ("how far") with "who counts".
`PLAN-returned-document.md` P01.S03; slice grill `memory/grills/2026-09-29-p01s03-refused-not-signer.md`, which
amended the plan-review pin. Closes the `State` half of /pending 737; answers /pending 736's consequence.
Supersedes ADR-059's two consequences that said B-LTA documents badge `Invalid` and refused records still count
as signers.

## Context

`sign.Verify` built `Status.Signers` from the LIBRARY's enumeration — one `SignerInfo` per signer
`digitorus/pdfsign` reported — and set `State = Invalid` for any that failed. After ADR-058 nib knew better
than that list: the sweep refuses records the library still enumerates, and marks document timestamps the
library reports as failed signers. Two shapes followed, both measured on `939c7986`:

- **A refused copy was a signer.** /pending 687's copied dictionary — the victim's `/Contents` and
  `/ByteRange` under a new object number — verifies in the library as a second signature by the victim. It
  read `valid, 2 signers`, and every reader that counts or positions signatures took it as one:
  `p2p.ContributionProgress` **halted the ceremony** (`ErrPrefixMismatch`, "signature 2 is (none)"),
  `NextPlacement` moved the next signature block from y=136 to y=232, `ReadAttestations` returned 2,
  `unverifiedSigners` counted it (withholding "Untampered" from a document signed only by a pinned
  counterparty), and the consent screen's `signersSoFar` listed a second row wearing the victim's name with no
  fingerprint. A copy the library hashes and FAILS turned a document whose every real signature verifies
  `Invalid`, and made the ceremony prefix unprovable.
- **A document timestamp was a failed signer** (/pending 737): the library reads it `ValidSignature=false`,
  so every PAdES B-LTA document read `invalid, 2 signers`.

The plan-review pin had already refused the obvious remedy — expressing a structural refusal as
`SignerInfo.Valid=false` — because that turns the document `Invalid`, reads as "the victim's signature was
tampered with", and halts a ceremony. The pin excluded "refused records"; the grill found that insufficient,
because no document timestamp is ever refused (ADR-059 removed the imprint cause), so /pending 737 would
have stayed open.

## Decision

**A signer is a record that is well-formed and is not a document timestamp** —
`Revision.countsAsSigner()`, `Cause == "" && !Timestamp`, the one predicate (ADR-009). `bounds()` is built
on it (`Verified && countsAsSigner()`), so "who counts" and "what bounds coverage" cannot drift. In
`verifyIndexed`, a library signer whose joined record does not count gets no `SignerInfo` and no vote on
`State`. Whether the library verified the record is not part of the predicate: a well-formed signature
whose hash fails is a signer, and it is the one that makes the document `Invalid`.

**A record is a document timestamp only when it is LABELLED one AND its blob ENCAPSULATES content.** The label
(`/Type /DocTimeStamp` or `/SubFilter /ETSI.RFC3161`) is text inside the signature's own coverage, so on its own
it let anyone take a FAILED signature out of the signers: Alice then Bob, Bob's `/Reason` tampered, reads
`invalid`; the same plus 13 bytes rewriting Bob's `/SubFilter` read `valid`, signers [Alice], "a document
timestamp is present". An RFC 3161 token encapsulates its TSTInfo and a detached signature encapsulates nothing,
so a labelled record whose parsed PKCS#7 has empty `Content` is a signer, and its failure sets `Invalid`. The
stronger test — `eContentType` equals `id-ct-TSTInfo` (1.2.840.113549.1.9.16.1.4) — is not available:
`digitorus/pkcs7` keeps the SignedData in an unexported field and exposes only `Content`. It would buy little:
a blob that encapsulates anything is attacker-written, fails as the signature it replaced, and removes that
signer exactly as truncation does, which `AddedAfter` reports.

**A refused record's failed verdict does not set `State`.** Every byte inside a counted signer's coverage is
hash-bound, so a refused record can hide only a change past the last counted signer — which truncation
already achieves and `AddedAfter` reports (measured: Alice then Bob, Bob's `/ByteRange` made odd in place,
reads `valid, [Alice], refused-signature-present`).

**Zero counted signers is never `Valid`, and is `Unsigned` only where no checkable blob exists.** It is
`Invalid` when `anyCheckableBlob` — the zero-library-signer path's own door — so a timestamp-only document,
and a lone signature refused in place, do not read "never signed". (A lone signature relabelled
`/SubFilter /ETSI.RFC3161` in place is not a timestamp at all — its blob encapsulates nothing — so it is a
signer that fails on its own rewrite.)

**`addedAfter`'s fourth argument stays the library's count** (`len(resp.Signers) > 0`), never
`len(st.Signers)`: its rule is "the library saw a signature and none bounds", and following the new count
would read a timestamp-only or refused-only document "nothing added".

**Under a join error nothing is excluded** (declared): the positions are not known to line up, so which record
a library signer is cannot be said. Every library signer is listed with no fingerprint, as before, and
`AddedAfter` warns `could-not-check`.

**What is not a signer is still reported.** `Status.Refused` (ADR-059) names every refused record;
`Status.Timestamps` lists the object numbers of document timestamps, which nib does not check. A refusal is
shown whenever there is one, independent of `AddedAfter`: the badge adds "a signature Nib refused is present"
(composing with "could not confirm", said once where the refused cause already says it), the details panel
names each refused object and each timestamp, and `nib verify` prints one line per refused record and per
timestamp and **exits 2 whenever a signature is refused**. `Invalid` with no signer and something
signature-shaped present is "no signature Nib could check", never "modified since signing".

## Consequences

- **`len(Signers)` counts people who signed**, so the co-sign placement index (`p2p/cosign.go`), the
  ceremony prefix (`ContributionProgress`), `ReadAttestations`, `Completeness`, `confirmCoSigned`,
  `unverifiedSigners` and `signersSoFar` are right on a copied dictionary with no change at those sites —
  each equals its answer on the untouched file (`TestARefusedCopyMovesNoCeremonyReader`,
  `TestARefusedCopyIsNobodyToTheServersSignerReaders`). This binds every reader of `Signers`.
- **A refusal can REMOVE a signer, never add or rename one** (/pending 736, accepted): a copy carries no
  fingerprint. A genuine signer removed by steering fails closed at `Completeness` (undischarged),
  `confirmCoSigned` ("missing") and `ContributionProgress` (their turn again). Counting a refused record whose
  fingerprint matches the roster was rejected — it is exactly how the copy would count. Nib's own signature
  dictionaries put `/Name`, `/M` and `/Reason` after `/Contents`, so they cannot be steered (measured).
- **Declared: a document carrying a refused copy places the next co-sign block differently under an old
  build and a new one** — only in that attack shape.
- **B-LTA documents now read `valid`**, one signer, "content added after signing" (the stamp's revision is
  an append, ADR-059), with the timestamp named. A cause that recognises a trailing timestamp revision needs
  DSS and imprint semantics and is /pending 737's residue. A timestamp-only document stays `Invalid` with the
  stamp named.
- **Install, mutation and undo gates that read `State`** now pass a B-LTA document and a document carrying a
  refused copy the library fails, as `valid` — intended.
