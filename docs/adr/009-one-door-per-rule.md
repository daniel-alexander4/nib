# ADR-009 — A rule gets one door, and its guard checks the door

**Status:** accepted
**Date:** 2026-08-20
**Context:** the v1.116.18 full-repo review and the sweep that worked it (v1.117.5–.14).
Generalises what ADR-008 did for one rule.

## Decision

When a rule about behaviour has to hold at more than one call site, it is written **once**,
as a named function, and every site calls it. Hand-mirroring the rule is not permitted, and
a guard that polices the rule polices **routing through the door** rather than the text each
site produces.

Where a site is deliberately exempt, the exemption is **named at that site**, with the
reason. A silent skip is indistinguishable from an omission.

## Why

This is the sweep's through-line and not a preference. Every one of these was a true
statement about *some* of the code:

| The rule | Sites it reached | What the gap did |
| --- | --- | --- |
| `ErrStampTextUnrepresentable` is the user's to fix | 2 of 3 | `StampPageNumbers` returned a 500, sending the user to look for a broken document instead of at the one character they could change |
| ADR-005's byte ceiling | 1 of 6 | five writers of `doc.data` grew documents past 512 MiB with nothing refusing |
| "this string names loopback" | 5 predicates, **3 different rules** | `loopbackOnly` requires two of them, and they disagreed about `127.0.0.2`, so a request each half separately called loopback was refused |
| refuse a vault from a newer Nib | 3 of 6 | `Migrate` decrypted and rewrote the file — the silent-downgrade path the refusal existed to stop |
| `document.name` | 3 of 5 producers | two documents rendered "Untitled" after a reload |
| the `[NibRoster:…]` format | 2 implementations | the **tested** one had no production caller, so the live one could change shape with nothing failing |

The counting is the point. A rule at *n* of *m* sites is not "mostly enforced" — it is a
sentence that reads as universal and is not, and the sites it misses are the ones nobody
thought about, which is the same population as the sites where it matters.

## Why the guard checks the door and not the text

`TestACommitFailureIsAlwaysA409` asserted the **status string** each of eight branches
printed. That is a check over eight copies of a rule, and it can only ever confirm that the
copies still agree — it has nothing to say about a ninth site added without one. Rewritten
to assert that every `commitMutation`/`commitBarrier` result is consumed by
`wroteCommitFailure`, it polices the property instead: the rule lives in one function, and a
call site that maps the error itself is a second copy of it.

Its **floor of eight is what caught the refactor** — it went red the moment the branches
stopped being branches. A guard over a population needs a floor, or a matcher that stops
matching reports full coverage.

## Consequences

- Doors introduced by this sweep: `wroteStampTextError`, `wroteCommitFailure`,
  `addrscope.Loopback`, `checkEnvelopeVersion` inside `readEnvelope`, `startingNodes`,
  `refuseAbsurdNesting`, `readNoFollow`, `parse` (already existed; two commands were
  bypassing it).
- A door may return more than one refusal. Two of these do, and both were `bool` before —
  a caller mapping one boolean onto two sentences cannot tell the user which happened.
- **This does not license collapsing a genuine platform split.** `setReuseAddr` and
  `oNoFollow` are build-tagged siblings *because* the underlying API differs; the rule there
  is that neither sibling is a stub and a real gap is declared in as many words
  (`nofollow_windows.go`). `internal/discovery`'s note once argued against such siblings —
  "a no-op sibling is the shape that already shipped one silent defect here" — and the
  result was that `nib.exe` could not be built at all. Right about the hazard, worse than
  the hazard.
