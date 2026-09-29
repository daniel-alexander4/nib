# ADR-059 — a document timestamp never bounds `AddedAfter`, and the refusal is the published answer

**Status:** accepted. Completes ADR-058's "how far" (its `/Fields` walk is deleted). `PLAN-returned-document.md`
P01.S02; slice grill `memory/grills/2026-09-29-p01s02-addedafter.md`, which amended the plan-review pins.

## Context

`Status.AddedAfter` warns that a document carries bytes no signature covers. Until P01.S02 it was computed
by its own walk over `AcroForm/Fields`, reading `/ByteRange` element 2 plus element 3 of every `FT /Sig`
field — whatever the field's `/Filter`, whether its signature verified, whether its byte range was the
revision around its own `/Contents`. /pending 661 is what that let through: an appended revision carrying
a signature-shaped dictionary claiming `/ByteRange [0 10 20 999999999]`, listed in `/Fields`, made the walk
read a coverage end past the file, and `nib verify` said `valid (1 signer(s))` with no warning about the
revision the decoy hid. Measured on the pre-S02 tree (`11490690`): both decoy shapes (no `/Filter`; PPKLite
with unparseable contents) read `valid, addedAfter=false`.

ADR-058 had already made `sign.Revision` the one home of who signed and whether a signature is well-formed,
and recorded each record's `CoverageEnd`, but left the walk in place. The plan-review then proposed that a
PAdES document timestamp (`/Type /DocTimeStamp`, `/SubFilter /ETSI.RFC3161`) bound coverage when its RFC 3161
`messageImprint` matches the hash of its ranges, so that honest B-LTA documents would not warn.

## Decision

**`AddedAfter` measures only records that verified, are well-formed, and are not document timestamps**
(`Revision.bounds`). The `/Fields` ByteRange walk is deleted; `coverage` over the records is the one
measurement, and `addedAfter` is `addedAfterVerdict`'s one caller (its fail-closed body unchanged).

**A document timestamp NEVER bounds coverage.** It names no signer, and a timestamp is obtainable by anyone
over any bytes: a public authority stamps whatever hash it is sent, and /pending 708 showed a self-minted
one reading as independent. "The imprint matches" would therefore let anyone hide appended content behind
a stamp. An honest B-LTA document reads `AddedAfter=true, appended` — accurate: the stamp's revision was
added after the last signature.

**The imprint is not checked on the verdict path at all, and a stamp is never refused on its account.**
The slice grill had it decide whether the record was refused (`timestamp-unverified`); the P01.S02 review
measured that and it is removed. It cost 11% of `Verify` at 10 MB for one stamp, and 1.98 s against 53 ms for
400 stamp records on a 5.9 MB document in the SWEEP — paid on every mutation, since every mutation
re-verifies. (That figure is the sweep's alone: the same document still costs ~6 s in `Verify`, because the
library hashes each record's ranges and nothing bounds the total across records — /pending 735, not this ADR's.) And
`digitorus/timestamp.Parse` refuses an honest RSASSA-PSS token, so the cause would have said "does not match
the bytes it claims" of a stamp that does. A stamp's record is marked `Timestamp`, bounds nothing, and is a
refused record only for a structural reason, as any record is. **The imprint is checked on demand by the
dispute surface (P02/P03, D10), never on the verdict path.**

**The answer is published with its reasons.** `Status.Refused` lists every refused record — object
number, `/Filter` (attacker-typed, capped at 64 bytes, escaped by every reader), cause — and
`Status.AddedAfterCause` says which fact set the bit, first match winning: `could-not-check` (the sweep or
the join could not be trusted), `could-not-check` again where no record bounds coverage — even with a
refused record present, because nothing measured an append — then `refused-signature-present` (a valid
signature bounds coverage, bytes follow it, AND a refused record exists), then `appended`. The review moved
the bounding check above the refusal: in the grill's order a refused record beside a sole failed signature
made the CLI and the badge claim an append nobody had measured. Where the cause is
`refused-signature-present`, every reader names BOTH facts — content added after signing, and a signature
Nib refused. A record with an empty `/Contents` (a prepare-for-signing placeholder) claims nothing and is
never listed in `Refused`. The CLI prints one line per refused record and the cause in its status line; the
badge and the details panel say the same, and `REFUSAL_WORDS` is held to the `RefusalCause` constants in both
directions (`TestEveryRefusalCauseIsSaidToTheUser`). **Nothing renders `AddedAfter` as "unchanged since you signed"** — that is P02's fingerprint-
selected prefix.

**The zero-signer path widens by the sweep's own rule.** Where the library reports no signer, `Verify`
answers `Invalid` if `signatureBlobPresent` OR any record carries a non-empty `/Contents` — EXCEPT a
well-formed one the catalog's `/Perms` names and the library never enumerated — so a `/Kids`-nested
signature no longer reads `Unsigned`. Not on "any record", which would turn a prepare-for-signing placeholder
(empty `/Contents`) from `Unsigned` into `Invalid`; and not on every blob, which turned a Reader-extended
form's intact `/Perms /UR3` signature with no `/SigFlags` into `Invalid` with no signer and no refusal to show
(found in review). **The exemption is `/Perms`, never "outside the library's enumeration"**: `/SigFlags` is a
catalog key an appended revision can drop, and exempting everything the library did not enumerate let anyone
strip it and turn a signed document `Unsigned` — hiding that it was ever signed (the re-review, measured on
four shapes).

**One enumeration, guarded.** `.Xref` (called or as a method value), and `.Key` given "ByteRange" or
"Fields" (as a literal or a constant) in non-test `internal/sign` — every declaration, not only function
bodies — appear only in `sweep`, or on a line marked `//sigwalk:exempt <name>` declared for that file:
`signatureBlobPresent` (re-expressed over the sweep it would narrow towards `Unsigned`) and
`hasCertificationSignature` (reads `/Reference`, never a `/ByteRange`; its `/Kids` blindness is /pending 734).
`revisions.go` uses no `bytes`/`strings` `Index*`/`LastIndex*`/`Contains*`/`Cut*`/`Count` and imports no
`regexp`; `.Key` is never taken as a method value; `addedAfterVerdict` is named once, in `addedAfter`.
(`TestEverySignatureEnumerationIsTheSweep`; each bypass the review found is driven red by
`TestTheSigwalkGuardSeesEveryBypass`.)

## Consequences

- **`AddedAfter` moves, in the safe direction, and that is not a regression.** Non-PPKLite and
  `adbe.x509.rsa_sha1` dictionaries, failed signatures and document timestamps no longer bound coverage, so a
  document whose last signature failed (already `Invalid`) now also warns `appended`, and every B-LTA document
  warns.
- **B-LTA documents still badge `Invalid`**, because the library reads a document timestamp
  `ValidSignature=false` and it sits in `Signers` as a failed signer (/pending 737). Excluding it from
  `Signers` is P01.S03's, and would unmask a coverage hole had this ADR let a stamp bound.
- **The verdict path pays nothing for a document timestamp** beyond reading its dictionary; the imprint's
  hash pass is the dispute surface's, on demand (D10).
- **A refused record that the valid signatures fully cover does not warn** — `AddedAfter` is a coverage fact
  — but it is listed in `Refused` either way, so a reader is still told.
- **Refused records still count as signers** until P01.S03: a verified copied dictionary is a second, unnamed
  signer in `Signers`, and the document's `State` is computed as before.
