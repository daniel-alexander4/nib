# ADR-156 — a peer turned away at the handshake is counted and named, and never ends the wait

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-009 (one door), ADR-149 (what a stranger may cost a listener). D19's cause 5
(`ClockSkewError`) is reused, not changed.
**Context:** `/pending 669` #3, decided by Dan 2026-10-12: *keep waiting, but name the cause*; and
`/pending 766` (2). A listener that refuses a peer at the pinned-peer check drops it silently on both
transports — `tlsListener`'s handshake goroutine returns, quic-go drops the connection inside the library —
and the armed screen showed a blank wait while the one expected peer was being refused over a wrong clock.
The dialler it refused saw a bare TLS alert under "could not connect". Ending the arm on a refusal was
rejected: any host that reaches the port can cause one, and one stale candidate would end a days-long wait.
**Applies:** `internal/p2p` (`refusal.go`, the four listener constructors), `internal/server` (`refused.go`,
the three arm goroutines, `connectFailure`, `writeConnectDiagnosis`, `raceCandidates`), `web/app.js`
(`reflectTurnedAway`).

## Decision

**1. A refusal never ends an arm and never spends it.** `IsTransportLoss` is unchanged: the ceremony arm
re-races as before. What changes is that the refusal is no longer silent.

**2. It is counted where the check runs, once.** `listenTLS` is the ONE door a listening TLS config comes
through; it wraps `VerifyPeerCertificate` and counts each refusal on a `p2p.Refusals` the listener hands back
(`Listener.Refusals()`, `HandshakeListener.Refusals()`). The check is inside the config both transports
share, so the count cannot differ between TCP and QUIC. `TestEveryListenerCountsItsRefusals` holds that no
constructor builds a server config any other way.

**3. Counted, never appended.** Integers, one time and one clock reading: no log line, no list, no
per-source table. A host hammering the port moves a number.

**4. Two causes are told apart, because the check tells them apart.** `verifyPinnedPeer` checks the pin,
then the leaf's signature, then its dates — so a clock refusal is reached only by a certificate the pinned
identity signed. *Not pinned* is anybody's to cause. A reading that the machine's clock is BEHIND needs a
leaf dated in its future and so cannot be forged; an *expired* reading can be replayed (a listener presents
its leaf to anyone) and never replaces a *behind* one.

**5. An arm that dials notes the mirror image on the same count** (`Refusals.NoteDial`): what answered was
not the pinned peer (`ErrPeerNotPinned`, now a sentinel), the pinned peer turned this machine away
(`RefusedByPeer`), or its leaf is outside this clock. **The far side's refusal is ONE signal:** TLS's
bad_certificate alert carries no text, so "not who I pinned" and "your dates are outside my clock" cannot be
told apart by the dialler, and its sentence says both. Under TLS 1.3 it arrives at the dial OR at the first
read, on both transports — measured — so both are read.

**6. One wording, by `refusalCause`, ordered by what a person can act on:** the clock; the peer turned this
machine away; something else answered; someone else connected here. The last never says the peer's identity
is wrong — only that someone who is not the pinned peer connected. A stranger's count is in the detail and
cannot displace a clock sentence.

**7. Shown at once, and given as the reason at the deadline.** `sessionStatus.turnedAway` is live — a field
of its own beside the diagnosis, not the sticky notice, so a refusal cannot overwrite a real failure — and
the window renders it politely. When an arm's wait runs out with nobody through, `noteWaitEnded` writes the
same cause to the sticky notice (`wait-ended-turned-away`).

**8. Every arm calls both doors.** The arms are three goroutines (`runSession`, `runCeremonyReceive`,
`armForDelivery`'s) and stay three: their lifecycles differ where it matters (accept-only on a `Listener`
with an expiry timer, against a dial-and-accept race on a `HandshakeListener` under a context deadline), and
merging them is not verifiable below tiers 4 and 6. `TestEveryArmNamesItsRefusals` takes the arms from the
source — every function that installs one — and fails on an arm that calls neither door, or on a fourth arm
it has not been told about.

## Consequences

- `p2p.Listener` gains `Refusals()`; a test double returns nil, which reads as nothing refused.
- A dialler turned away is told so (`dialRefusalSentence`) at the send route, the hop route's diagnosis
  (`cause: turned-away`) and `connectFailure`; in a lost race a named refusal outranks an unnamed loss, and
  a clock disagreement still outranks both.
- Nothing on the wire changes, and nothing a peer on another build reads.
- **Declared:** a peer that presents no certificate, or never speaks TLS, is not counted — it presented
  nothing to be wrong about. A LISTENER cannot see that a dialler's own pin refused it: QUIC hides it and
  over TCP it is an alert anyone can send, so only the dialler names that. quic-go's own handshake work for
  an unvalidated source is not bounded by this (`/pending 766` (4)).
- **Refused:** ending the arm; a log line per refusal; a per-source table; putting the live line in the
  sticky notice; a different sentence per transport; telling the dialler which of the two refusals it met.
