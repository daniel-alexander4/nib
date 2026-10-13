# ADR-135 — the convener signs their own turn through one local route

**Status:** accepted
**Date:** 2026-10-12
**Extends:** `PLAN-signing-ceremony.md` D22 (the convener is the hub of every hop). ADR-001/004 (pinning),
ADR-008, ADR-009, ADR-012, ADR-013.
**Context:** /pending 869. Under D22 the convener dials every hop and `POST /api/ceremony/hop` targets the next
signer, so at the convener's OWN turn there was nobody to dial — and no other route the window calls signed.
`/api/cosign/sign` and the window's `/api/session/initiate` refuse a convened document. Confirmed in a real
window: with the convener signing first (the setup sheet's default) the panel drew "It is your turn to sign
(1 of 2 signing)." with no control, and nothing moved the ceremony on. The multi-process tiers completed such a
ceremony only through `/api/session/initiate` with an invitation, a call no window makes. The signing door itself
(`buildCoSigned`) worked at any position.
**Applies:** `internal/server/ceremonysignturn.go`, `ceremonyhop.go` (the quote), `web/app.js` (the rail).

## Decision

**1. `POST /api/ceremony/sign-turn` is a hop with the dial taken out.** It signs the pinned document only when
the roster's next signer is this machine AND this machine convened. Before the signature it runs the hop's own
functions — `hopTarget`, the `holdHop` slot, `checkArrival`, `checkCeremonyDeadline`, `carries`, then
`buildCoSigned`; after it, `installCeremonyResult` and then `mirrorHop`, before the response. A party who did not
convene is refused (403) and still waits to be called.

**2. Its block is quoted by `GET /api/ceremony/hop`** — at the convener's own turn the quote carries the block's
lines, rect and pinned time. No second quote route.

**3. What the signature accepts is the roster's predecessor**, never a wire peer (there is none); empty for a
first signer.

**4. The window offers one button, *Sign now*, on `isMe && convenedHere`,** with the permanence statement beside
it before the press. One press signs. A document opened cold is adopted as the ceremony's tab, so the result
replaces it instead of opening beside it.

## Consequences

- A signing convener's ceremony can be run from the window at any roster position: first, middle or last.
- Refused: riding the convener's block on the next dial (one press would both sign and call, and it cannot serve
  a convener who signs last), and unticking "I sign" by default.
- Declared: there is no block preview before the press — no initiating flow has one to reuse. Do not add one
  here without designing it for the receiving consent screen too.
- Do not let the route take a document it did not capture at its start.
- Not driven in a browser beyond the first-of-two position; a sole-signer roster is untested; no multi-process
  tier presses the button (they still sign through the dial).
