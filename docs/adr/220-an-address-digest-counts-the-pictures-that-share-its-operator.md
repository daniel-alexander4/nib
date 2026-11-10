# ADR-220 — An address's digest counts the pictures on its page that share its operator

Status: accepted, 2026-11-09 (PLAN-image-edit P01 phase-close review, finding C1).
Extends ADR-219 §2.

## Context

ADR-219 names a picture by (page, image-only ordinal, digest), where the digest covered the operator's bytes, the
`cm` that places it and the `q` depth, and the route echoes no `baseRev` because "the object digest is the address
check". Two identical draws on one page share that digest and differ only by ordinal. Once ordinal 0 is removed, its
twin becomes ordinal 0 with the same digest, so the removed picture's address names the twin. A second press of
Remove from page while the first was in flight, or the same `nib objects remove … -w` run twice, removed both. The
door's after-check could not see it: the page really was the original minus one picture.

## Decision

1. **`PageObject.Digest` is the address's digest** (`addressDigest`): SHA-256 under its own label over the operator's
   digest (`objectDigest`, unchanged) and the number of pictures on the page that share it. Removing one of N twins
   leaves the others with a count of N-1, so every twin's address goes stale and a replay is `object-changed`. A
   picture with no twin keeps its address across another's removal.
2. **What a write is checked against is the operator's digest**, kept beside it unexported (`PageObject.operator`):
   the after-check (`rewriteAgrees`, `pageAsItStands`) and the corpus oracle compare operators across the write,
   because removing a twin moves the survivor's address and not its operator.
3. **The window sends one removal at a time per view** (`objectsRemoving`), and on `object-changed` it loads the
   document back where it moved (`reloadIfMoved`, ADR-146) or forgets the stale listing and reads the page again.
   The door's refusal is the guarantee; the window's guard only keeps it from asking.
4. Still no `baseRev`: ADR-219 §2 stands, now with an address that is true to its claim.

## Consequences

- Held by `TestAReplayedAddressDoesNotRemoveTheOtherOfTwoIdenticalPictures` (red with the count taken out) and, in
  the window, `editobjects.test.mjs`'s in-flight and stale-address tests (each red without its fix).
- An address is good only for the page as it was listed; that was ADR-219's intent and is now its mechanism for twins.
- Nothing persisted carries an address; the digest's change of meaning moves no stored value.
