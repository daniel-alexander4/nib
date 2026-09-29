# ADR-058 — a signature is one record, and the library is joined to it by position

**Status:** accepted. Supersedes ADR-051 in part (its join key). `PLAN-returned-document.md` P01.S01.

## Context

ADR-051 made a signer's identity the certificate its `SignerInfo` names, and re-parsed every blob over
the library's own xref sweep to find it. Because `verify.Signer` carries nothing that identifies which
dictionary it came from, the re-parse was joined back to the library's signers by a **map keyed on the
certificate bag**, contents and order, and a bag two signatures shared but disagreed about was blanked.

P01 needs more than identity from that sweep: per signature, *how far it reaches* (the `/ByteRange`'s
coverage end) and *whether it is well-formed* (the eleven-conjunct structure rule that refuses a copied
dictionary, /pending 687), next to *who signed*. Keeping those three in three places is the defect
ADR-009 exists to refuse, and the bag map could not carry them: it is keyed by a value the attacker
writes, it holds one answer per bag rather than per signature, and its ambiguity rule blanked two honest
signers whenever their bags happened to coincide.

Two facts make a better join available. The sweep and the library are **the same enumeration** —
every xref object, `/Filter /Adobe.PPKLite`, in xref order, a `pkcs7.Parse` failure skipped
(`pdfsign verify/verify.go:87-101`), nothing reported without `/SigFlags` (`:81-84`). And the library
returns an **empty** bag on every failure path (`signature.go:36-63`; the bag is filled only at
`certificate.go:345`, after `ValidSignature` is set), so a bag is present exactly where a signature
verified.

## Decision

**Each signature-shaped dictionary is one `sign.Revision` record, and it is the one home of who signed
and whether it is well-formed.** `sweepRevisions` builds the records; `SignerInfo.Fingerprint` is read
from the record at the signer's position, and no other code computes it
(`TestTheSignerFingerprintHasOneWriter`). **"How far" is recorded here too (`CoverageEnd`) but is not
yet its one home**: until P01.S02 deletes it, `trailingContentAfterLastSignature`'s `/Fields` walk still
computes coverage for `AddedAfter` on its own.

**The library is joined to the records by POSITION**: ordinal i among the records the library would
process — PPKLite, parseable, `/SigFlags` present, xref order — is the library's signer i. **The bag is
the cross-check, not the key**: the count must agree, a non-empty library bag must equal its record's,
and an empty library bag must come with `ValidSignature=false`. Any disagreement is an error, routed
fail-closed — every fingerprint blank, `AddedAfter` set.

ADR-051's identity rule is unchanged: the certificate the `SignerInfo` names, `GetOnlySigner`, and no
fingerprint where that cannot be established — now also none where the signature did not verify, and
**none on a structurally refused record even where it did**. A copied dictionary (/pending 687) verifies
as the victim, and P02 selects records by fingerprint; a refused copy carrying the victim's fingerprint
would be selected as the victim.

**A per-signature re-verification was refused.** A record could carry nib's own `p7.Verify` and need no
join at all; measured by the plan-review, the library is 13-23 ms per signature at 10 MB and the sweeps
under 3% of `Verify`, so a second hash of every signed prefix would roughly double the dominant cost on
a path that runs on every install, mutation and undo.

## Consequences

- **Two signatures sharing a bag are each named** (`TestOneBagWithTwoSignersNamesEach`). ADR-051's
  "an attacker can suppress a fingerprint by building two blobs that share a bag" no longer holds; the
  ambiguity was an artefact of the map.
- **A shift is detected, not absorbed.** A record the library does not see, or a library signer with no
  record, fails the count; a library bump that changes the predicate fails it too. The bag check covers
  every position that verified — the only ones that bound coverage or anchor an identity.
- **The sweep runs before the library**, behind ADR-041's readability gate, so a `/ByteRange` the
  library must not be handed — an indirect array (re-parsed three times per pair: 7.86 s for 4,000
  pairs on a 29 KB file), a negative length (`io.NewSectionReader` reads it as "to the end", so
  `[0 -1 0 -1 …]` copies the file per pair), or clamped reads summing past the file (`[0 S 0 S … ×K]`,
  which the library would copy K×S of) — is refused `Invalid` without the library being called. Shapes
  that read nothing — an odd count, a negative offset, a pair starting past EOF — are NOT gated: they
  are refused RECORDS, and the document keeps its co-signers' verdict. A sweep that cannot finish is `Invalid` before the
  library too, with `AddedAfter` set. Ungated, the sweep is a digitorus parse like any other and
  spins on the same damaged object streams ADR-041 measured.
- **`Revisions(pdf)`** exposes the records through the same door `Verify` uses. A record outside the
  library's enumeration is unverified by definition, never by index.
