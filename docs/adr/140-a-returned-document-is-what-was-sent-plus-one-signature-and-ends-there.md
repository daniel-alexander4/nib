# ADR-140 — a returned document is what was sent plus one signature, and ends there

**Status:** accepted
**Date:** 2026-10-12
**Supersedes:** ADR-060 §Consequences, first bullet, **for a RETURNED document only** — the clause that binds
`confirmCoSigned` to answer on a document carrying a refused copy as it does on the untouched file. Every other
reader that bullet names, and every reader of a document that is merely opened or that ARRIVES, stays as ADR-060
has it.
**Extends:** ADR-058 (one record per signature), ADR-059 (the three `AddedAfter` causes), ADR-009 (one door).
**Context:** /pending 629, decided by Dan 2026-10-12. A hop sends a document and takes one back. The two
verbs that take one back — `p2p.Initiate` (through `confirmCoSigned`) and `p2p.Carry` — checked that the
bytes grew from what was sent and that the right party's valid signature was on the part that grew. Neither
asked how MUCH grew, and since `8843a7df` they refused two of the three coverage causes. Four returns passed
every check:

- **A refused copy after the peer's signature.** Content plus a signature-shaped dictionary nib refuses reads
  `refused-signature-present`, which ADR-060 bound `confirmCoSigned` to accept.
- **A third key's valid signature after the peer's.** `AddedAfter` is bounded by ANY valid signer, so the
  document has nothing after its last signature and both parties' signatures verify.
- **The next TWO rostered signers answering one carry.** The chain is the roster's own, in order and
  cross-bound, and it advanced — by two.
- **A carry to the wrong party.** `Carry` never compared the peer on the wire with the party whose turn it is:
  a carrier connected to any pinned ceremony member sent it the document, and accepted a reply carrying the
  right party's signature from the wrong one.

**Applies:** `internal/p2p/session.go` — `returnedWithOneSignature`, `nothingAfterTheLastSignature`,
`confirmCoSigned`, `Carry`.

## Decision

**1. One door says what a return may be: `returnedWithOneSignature`.** `Initiate` (through `confirmCoSigned`)
and `Carry` both ask it, with the verification of what was sent, the verification of what came back, the
length of what was sent, and the fingerprint of the party the hop was for. Four facts, in this order:

- nothing follows the last signature — **all three** `AddedAfter` causes are refused, each in its own words
  (`appended`, `refused-signature-present`, `could-not-check`);
- every signature that was sent is still there as it was sent: the signers whose coverage ends inside the sent
  bytes are the sent document's signers one for one — same signer, same reach, same verdict, same order;
- **exactly one** signature reaches past the sent bytes;
- it verifies, and it is the party's the hop was for.

**2. "Last" is measured, not positional.** `Signers` is in the library's object-number order, which says
nothing about time; `CoverageEnd` does. With nothing after the last signature some valid signature reaches the
end of the file, and every signature but the new one ends inside the sent bytes — so the new one is last.

**3. In `Initiate` the party is the wire peer; in `Carry` it is the roster's next signer, AND the wire peer
must be that party** (`ch.PeerFP` against `NextContributor`), asked before the spoken check and before a
document byte is written. The refusal is `ErrNotYourTurn`. The receiving side's own turn rule is an honest
receiver's refusal; this one does not depend on the other end running it.

**4. A refused record INSIDE the last signature's coverage is not refused here.** It sets no coverage cause
(ADR-059), and the sent document may honestly carry one that arrived that way. The return rule is about what
the hop ADDED.

## Consequences

- `TestARefusedCopyMovesNoCeremonyReader` loses its `confirmCoSigned` row; the other six readers keep theirs.
  `TestACoSignedReturnStopsAtTheLastSignature` now refuses the refused copy it used to pin as an exemption.
- `confirmCoSigned` verifies the SENT document once more per exchange (one hash per signature already on it),
  after the peer has answered — on a path that has just waited for a person. `Carry` verifies what it sends
  once, where `NextContributor` already did.
- **Declared: a peer on an older build is unaffected on the wire** — nothing sent or expected changes; only
  what this side will keep. An honest return from any build is one signature and ends there.
- **Declared: a hop that legitimately returns two signatures does not exist and cannot be added without
  superseding this.** Counterparts signed in parallel and merged are a different verb, not a wider return.
- Not asked of an arriving document: content between signatures is expected there, and the signature the
  receiving party is about to make covers what it was shown.
- The multi-process tiers (4, 4 `-n 4`, 6) are what observe that no honest hop is refused across real
  processes; `TestEveryHonestRelayShapePassesTheReturnDoor` walks the same shapes in one process.
