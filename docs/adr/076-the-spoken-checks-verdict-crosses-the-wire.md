# ADR-076 — the spoken check's verdict crosses the wire

**Status:** accepted. `/pending 802` (2026-10-03), from the v1.179.5 P02 phase-close review of
`PLAN-returned-document.md` (R7). Session protocol `nib/3` → `nib/4`; extends ADR-028's negotiation.
Reverses the "no wire code" decision recorded on `p2p.ErrVerificationDeclined`.

## Context

After the four words, each side's user confirms or rejects them. A side whose user did not confirm returned
`ErrVerificationDeclined` (or `ErrVerificationTimedOut`) and its caller closed the connection — no byte. The side whose
user DID confirm then failed its next I/O: measured over loopback TCP, `send document: write: broken pipe`, and over
QUIC `receive co-signed document: EOF` or `Application error 0x0 (remote)`. `IsTransportLoss` whitelists exactly those,
so the server re-raced (the glare dial loop) or re-accepted (the ceremony arm) a session the other person had just
rejected, and its user was never told the words were rejected — the one outcome the whitelist exists never to retry.

The silence was deliberate: the sentinel's doc argued that telling the peer which check failed hands a
man-in-the-middle "whether the human noticed". It does not withhold that. The peer has just completed a
commit-and-reveal with this machine, so a close straight after the gate is the answer, and an attacker reads it
with or without a byte. What the silence withheld was the same fact from an honest peer.

## Decision

On `nib/4`, after its own gate each side writes a one-byte VERDICT — `ackOK`, or `ackWordsNotConfirmed` for every way
of not confirming (declined, timed out, gate not shown) — and a side that confirmed waits for the peer's before any
document byte moves. The confirming side decodes a negative verdict as `p2p.ErrPeerDidNotConfirm`, an ordinary error
that falls through `IsTransportLoss`. The server lifts it before every network fallthrough: a 409 sentence on
`runHopDial` and `/api/session/send`, and a `words-not-confirmed` status notice on the receiving arm.

- **One code for decline and timeout.** The half of the old reasoning that stands: the pair stays indistinguishable
  on the wire, so the code is not a report of whether a person was at their screen.
- **Negotiated, never unconditional.** The verdict is a frame in both directions; a `nib/3` build would read the
  dialer's verdict as its document. `Channel.SpeaksVerdict` is a floor that fails closed, as `SpeaksRoleFrame` is. An
  older peer gets the pre-verdict behaviour unchanged.
- **The verdict is read CONCURRENTLY with the local gate.** quic-go's `CloseWithError` abandons unacknowledged data and
  a stream that has seen the close returns the close error ahead of buffered data, so a verdict left unread while a
  user deliberates is destroyed by the decliner's close. A side that did not confirm also lingers for the peer's
  verdict or close, bounded by `closeGrace`, so the frame is consumed before its connection closes. Both are measured:
  without the concurrent read, a receiver answering after the linger reads a QUIC application error; without the
  linger, every dialer-declines case over QUIC does.
- **No new deadline arm.** The two gates run concurrently from the end of the exchange, each bounded by
  `PeerGateWindow`, so the peer's verdict lands inside the `exchangeDeadline` the entry point already armed for "the
  spoken verification gate". `SessionBudget`, `DeliveryLegBudget` and `ReceiveArrivalLag` are unchanged.

## Consequences

- A confirming dialer no longer sends its document to a peer whose user has not yet confirmed.
- A side that did not confirm can take up to `closeGrace` (5 s) longer to report its own outcome when the other user
  is still deliberating; it returns at once when they have already answered.
- **Declared gap:** under a man-in-the-middle the confirming side's peer IS the attacker, who may withhold the verdict
  and close; that side then sees a transport loss and re-races into a fresh spoken check, exactly as before. The
  verdict helps the honest case and cannot be relied on in the adversarial one, which is why it is not a security
  signal and the four words remain the only check anchored outside the channel.
- `TestEveryALPNConfigSiteOffersTheSameList` fired, as it is meant to; the offer list is `nib/4, nib/3, nib/2, nib/1`.
