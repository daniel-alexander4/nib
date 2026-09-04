# ADR-013 — `DocHash` is a hop-1 anchor, and `ContentDigest` keeps its annotations

**Status:** accepted
**Date:** 2026-09-04
**Context:** `/pending 358`; `internal/pdfops/attachments.go`'s `ContentDigest`;
`internal/ceremony/record.go`'s `Record.DocHash`; the three gates that disable the
comparison past the first signature.
**Applies:** the whole ceremony path. It constrains what may be built on `DocHash`,
which is why it is a decision record rather than a comment on any one of the gates.

## Decision

**`Record.DocHash` is verifiable only before the first signature. After it, the
signatures — not `DocHash` — are what bind a party to bytes.**

`ContentDigest` covers each page's `/Annots` in full, and a visible signature adds a
widget annotation. So from the first signature onward the digest legitimately moves and
no party can recompute it. That is a property of the format, not a hole in a check, and
three gates already encode it, each for its own caller:

- `CheckDocument` (`internal/ceremony/embed.go`) — its own doc comment states that a
  receiving party can never pass it on an arrival carrying a signature.
- `ReadMirror` (`internal/ceremony/mirror.go`) — the comparison is gated on
  `sign.Verify(pdf).State == sign.Unsigned`, because an unconditional check would refuse
  every honest mirror from hop 2 on.
- `checkArrival` (`internal/server/ceremonyid.go`) — gated on
  `!sign.HasSignatureBlob(pdf)`, the only comparison against a document arriving from a
  counterparty, and it fires exactly once per ceremony: at hop 1.

**`ContentDigest` is not changed to make the comparison survive signing.** That is the
decision this record exists for, and it is a refusal.

## Why the alternative was rejected

The obvious remedy is a content digest that excludes the annots a signature adds, making
`DocHash` checkable at every hop. It was rejected on a measured price, not on taste.

**The annots term is not incidental: it is there because its own exclusion was refuted, and
the annots block in `ContentDigest` names what came back.** The argument for excluding it was
*"everything else is covered by the signatures"* — and the window this digest is checked in
is the pre-FIRST-signature hop, where there are none. In that window Nib's own operations
defeat a content-only digest: `AddNotes` writes `/Text` annotations and the form fill writes
`/V` plus widget `/AP` streams, both inside `/Annots`, and neither touches a content stream.
**For a contract, the form values are the agreement.**

**The same argument was made once more, about attachments, and measured false.** At the
P07.S02 grill an attached `Schedule-A.txt` reading *"rent is 1000/mo"* was removed and
re-added under the same filename reading *"rent is 100000/mo"*; the digest did not move and
`CheckDocument` returned nil. That exclusion is now closed by `hashEmbeddedFiles` (v3), so
the swap is covered today — it is cited here as the precedent, not as a live hole: twice the
"the signatures cover it" reasoning was applied to a class, and twice it was wrong in the one
window where there is no signature to fall back on.

So the trade is not "a slightly weaker digest for a checkable one". It is: make `DocHash`
checkable after hop 1 **at the cost of reopening sticky notes and form values in the
pre-signature window** — the window `checkArrival` exists for, and the one where the
signatures are not the fallback. Both sides are real and neither is obviously smaller.

**And the change is not free even where it is right.** `ContentDigestVersion` is bound into
the digest precisely because altering what is covered moves every hash: at v1.116.18 a
coverage change made a record written by the previous build fail the comparison with *"these
are not the same document"* when the cause was a Nib point release. A signature-stable digest
is therefore a version bump with a skew story, not an edit to one function.

## What remains, stated exactly

A convener who is **also the first signer** can end a proceeding on a document whose bytes
are not the ones its record commits to, and nothing after hop 1 can say so. Three things
bound that, and none of them is a check on `DocHash`:

- **A different RECORD is already refused.** `RosterHash` commits to the format version,
  the convener, the id, `DocHash`, the digest version, the intent, the deadline and every
  roster entry, and `MatchesRecord` compares it. Two documents carrying two different
  records produce two different roster hashes and the invitation refuses one of them —
  P07.S02's hardening, and it closes the two-chains attack.
- **Every signature covers the bytes it signed.** A substitution between hops breaks a
  signature unless the convener re-signs, which is why the residual is the
  convener-as-first-signer case and not the general one.
- **Every party sees the document they sign.** The consent screen renders the arriving
  document and lists every signature already on it, so a party's assent is to the bytes in
  front of them.

What is lost is narrower than "the document is unverifiable": it is that the record's own
statement about which bytes the proceeding is over cannot be confirmed by anybody after
hop 1, including a reader of the finished document years later.

## What this binds

- **Nothing may be built that reads `DocHash` as a post-hop-1 anchor**, and no surface may
  report a record-match it has not performed. Checked when this was written: no surface
  does. The verifier's user-facing language is per-signature — `✓ Untampered` /
  `⚠ Modified since signing`, plus the `addedAfter` report — and it is accurate about what
  it says. A record caveat was considered and not added, because it would fire on every
  signed document, and the majority carry no ceremony record at all.
- **A fourth gate is not the remedy.** Adding another site that disables the comparison
  restates ADR-009's rule rather than applying it; the three above are one rule reaching
  three callers, and this record is its written form.
- **`CheckDocument` keeps its zero production callers on purpose.** `checkArrival` inlines
  the byte comparison instead, because `CheckRecord` has already re-verified the convener
  signature by then and the full call costs 13.8 s against 8.7 s on a 4.4 MB document.
