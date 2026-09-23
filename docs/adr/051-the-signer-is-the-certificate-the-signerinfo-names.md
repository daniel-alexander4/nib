# ADR-051 — the signer is the certificate the SignerInfo names, never the one that leads the bag

**Status:** accepted

## Context

A PDF signature's `/Contents` holds a PKCS#7 `SignedData`. That structure carries two separate
things: a `certificates` field — a **SET OF** certificates, the "bag" — and one or more
`SignerInfo`s, each of which names the certificate it was made with **by issuer and serial number**.
The bag is a set: it has no order in the abstract, and the order it happens to be encoded in
means nothing.

Nib read the signer's identity out of the bag's first element:

```go
// Nib identities are self-signed single certs, so element 0 is the signer.
if len(s.Certificates) > 0 && s.Certificates[0].Certificate != nil {
    si.Fingerprint = hex.EncodeToString(fingerprintOf(s.Certificates[0].Certificate))
}
```

The comment is true of documents **nib produced** — `digitorus/pkcs7`'s `AddSignerChain` appends
the signing certificate before its parents (`sign.go:194-197`) and `marshalCertificates` does not
re-order (`sign.go:412`). It is a statement about nothing at all for a document that arrived from
somewhere else, which is the only kind this question is ever asked about.

Three facts make the gap exploitable rather than merely untidy:

1. **The bag is not signed.** `/Contents` is the hole in the `/ByteRange`; every byte of it,
   including the certificates, sits outside what any signature covers. Reordering it needs no key
   and leaves every signature verifying.
2. **Verification does not care about position.** `p7.Verify` resolves the signing certificate with
   `getCertFromCertsByIssuerAndSerial(p7.Certificates, signer.IssuerAndSerialNumber)`
   (`digitorus/pkcs7 verify.go:106,163`) — by name, not by index.
3. **`verify.Signer` reports the bag and discards the name.**
   `buildCertificateChainsWithOptions` appends every certificate in `p7.Certificates` order
   (`digitorus/pdfsign verify/certificate.go:128,345`) and keeps no issuer-and-serial anywhere in
   the result. The library's own answer to "who signed" is unreachable from its output.

So a signature made with an attacker's key, carrying a victim's certificate first in the bag,
verified `Valid: true` under the **victim's** fingerprint. Measured, not argued:
`TestAForgedBagDoesNotRenameTheSigner` builds exactly that document and it reported the victim.

That fingerprint is not decorative. `p2p.Attestations` copies it onto each attestation
(`attestation.go:561`); `p2p.Completeness` counts an obliged party as having signed on
`a.Valid && EqualFold(a.Fingerprint, want)` (`attestation.go:542`); and `unverifiedSigners`
(`server/server.go:1780`) treats a fingerprint in the pinned-peer set as a recognised signer. A
forged "X co-signed" therefore reaches the roster, the badge and the completeness verdict alike.

**Two certificates in one bag is the ordinary shape, not an exotic one.** `SignExternal` embeds a
leaf and its CA chain on every imported-identity signature, so this is not a question that only
arises under attack — a conforming producer that emits the SET in DER-canonical order rather than
signer-first would misattribute an honest signature.

## Decision

**A reported signer identity comes from the certificate that signature's `SignerInfo` names, by
issuer and serial. Where nib cannot establish which certificate that is, there is no fingerprint.**

Nib re-parses each signature's PKCS#7 itself (`signerFingerprintsByBag`, `internal/sign/signercert.go`)
and takes the fingerprint from `p7.GetOnlySigner()` — which resolves through the very call
verification uses, so the certificate reported is by construction the one whose public key checked
the bytes. Three consequences follow, and each is the fail-closed direction:

- **A bag naming no signer nib holds, or carrying more than one `SignerInfo`, yields no
  fingerprint.** `GetOnlySigner` is nil for both, and an empty fingerprint is what nib says when it
  cannot say who.
- **Two signatures sharing one bag but not one signer yield no fingerprint for either.** The bag is
  the join key (below); an ambiguous key is blanked rather than guessed.
- **An empty fingerprint discharges no obligation.** `Completeness` skips an empty roster entry and
  an empty attestation, because `EqualFold("", "")` is true and two absences must not agree.

**The join key is the certificate bag, contents and order.** The library's result cannot be matched
to a re-parsed blob by index or by any field it exposes: `Name`, `Reason` and the timestamp are all
attacker-supplied, and the issuer-and-serial that would identify the signature is exactly what
`verify.Signer` discards. The bag is the one value both sides hold and it round-trips byte for byte.
Order is part of the key: two bags differing only in order are two different bags, and matching
across them would re-introduce the assumption this ADR removes.

**Both sides of that key must come from the SAME enumeration, and this is the part that was got
wrong first.** The library finds signatures by resolving every object and testing it for `/Filter
/Adobe.PPKLite` (`pdfsign verify/verify.go:84-93`); it never reads `AcroForm/Fields`. The first
implementation walked `/Fields` because it is two orders of magnitude cheaper, on the reasoning that
a signature the cheap walk misses simply gets no fingerprint — fail-closed, since the map is only
read by key and issues no all-clear.

**That reasoning is false, and the counterexample is the original attack with one extra step.** The
bag is unsigned bytes in the `/Contents` hole, so the attacker writes *both* sides of the key. Two
walks over different object sets let them be written separately: author the real signature —
`/Filter /Adobe.PPKLite`, the attacker's key, bag `[attacker, victim]` — reachable through the xref
but absent from `/Fields`, and add a decoy `/Fields` entry with `/FT /Sig` whose `/V /Contents`
carries the same two certificate DERs in the same order, a `SignerInfo` naming the victim, and no
`/Filter` key so the library skips it. The cheap walk keys the bag from the decoy; the library's
signer looks the same bag up; the victim's fingerprint comes back on a `Valid` signature. A gap in
the walk does not mean that no object supplies the key — it means a **different** object does.

So nib performs the library's own walk. With one enumeration, any second blob carrying the same bag
is one the library also reports, so it lands in the same map: same bag, two signers, and the entry
is blanked rather than believed (`recordSigner`). `TestASignatureAbsentFromFieldsIsStillAttributed`
pins the choice of walk and goes red against the `/Fields` one.

**The cost is the sweep's cost, paid deliberately.** Measured: **18.7 ms** on a 400-page signed
document beside the **20.0 ms** `verify.Verify` call it follows, 4.9 ms at 100 pages, 201 µs at one
page — 0.6× to 1.0× that call, because it is the same sweep. Whole-`Verify` totals over the same
fixtures: 781 µs, 10.4 ms, 44.8 ms. Measured range 1 to 400 pages, 3 KB to 79 KB. The cheap walk
would have been 176 µs at 400 pages; **that saving is what bought the hole**, and it is recorded
here so the trade is not re-proposed as an optimisation.

## Consequences

- **A third-party signature whose bag nib cannot resolve loses its displayed fingerprint** where it
  previously showed element 0's. That is the point: it was showing a value it had not established.
  The badge, `unverifiedSigners` and `markUnrostered` all already treat an unidentified signer as
  unrecognised.
- **An attacker can suppress a fingerprint** by building two blobs that share a bag and name
  different signers. That converts a recognised co-signer into an unrecognised one — an
  under-report, which surfaces as ⚠ incomplete rather than as false reassurance. It is the price of
  refusing to guess between two candidates, and it is the safe direction.
- **A signature that does not verify has never been attributable, and still is not.**
  `processSignature` returns as soon as `verifySignature` fails and never reaches
  `buildCertificateChainsWithOptions` (`pdfsign verify/signature.go:50-61`), so such a signer is
  reported with an EMPTY certificate bag — there is no bag to key on and the old element-0 read
  found nothing either. Unchanged by this ADR, recorded because it is the obvious next question and
  the answer is a property of the library rather than a choice made here.
- **`SignerInfo.Name` is not evidence and never was.** It is the signature dictionary's `/Name`
  (`digitorus/pdfsign verify/signature.go:19`) — text the signer typed — not the certificate's
  subject, which this field's own comment claimed until now. `Fingerprint` is the identity.
- **`github.com/digitorus/pkcs7` becomes a direct dependency.** It was already in the build as
  pdfsign's own; nib now parses the same blob with the same parser rather than a second one, so the
  two cannot disagree about what the bytes say.
- **`signerInfo` takes the map as a parameter.** A call with `nil` reports no fingerprints, which is
  what makes the fail-closed path the default rather than an error case.
