# ADR-128 — a launch believes only an instance that proves the token

**Status:** accepted
**Date:** 2026-10-10
**Extends:** ADR-006 (the hand-off credential), ADR-054 (`GET /api/instance` is outside the session door and
carries its own secret). Neither decision changes.
**Context:** /pending 827. A launch reads the instance record, asks the recorded address whether a Nib is
there (`instance.Probe`), and on *alive* sends that address the hand-off secret and the document's path. The
probe sent the record's token and believed any HTTP 200. That authenticated the launch to the listener and
never the listener to the launch: after a crash, whatever bound the freed loopback port — another user's
process included, since loopback is not per-user — answered 200 and was handed the secret and the path.
`internal/instance/instance.go`, `internal/server/server.go` (`handleInstance`), `cmd/nib/main.go`.
**Applies:** the instance probe and anything that later reads its verdict.

## Decision

**1. The answer proves the token, not only the question.** A probe sends a fresh nonce
(`X-Nib-Instance-Nonce`) and, in `X-Nib-Instance`, an HMAC-SHA256 of the nonce keyed by the record's token —
never the token itself, because a listener handed the token could compute any answer. The instance checks
that proof before it computes anything, and answers with its own HMAC over the same nonce. The probe is
*alive* only on an answer carrying the matching proof.

**2. The two directions use different labels**, so the proof a probe sends can never be handed back to it as
the answer it is waiting for.

**3. Anything else that answers is *gone*.** The record is cleared and the launch becomes the primary; nothing
is handed to the listener. A timeout stays *unknown*, as before.

**4. Compatibility is a marker in the record, read from the record alone.** A record a new build writes says
`challenge: true`; one without it was written by a build that can answer nothing else and is probed the old
way, so a launch during an upgrade still reaches the running old Nib. The marker is never read from the
answer: whoever holds the port cannot write a 0600 file in the user's configuration directory, so cannot ask
for the weaker probe. The server still accepts the token itself when no nonce is sent, for an older build
launching into a new one.

## Consequences

- One HMAC per launch probe, on no request path a window uses.
- **Declared gap:** a record left behind by a crashed build that predates this stays exposed to the old
  probe until it is cleared once — by a launch that finds nothing there, or by the next clean exit. Refusing
  the old probe outright would make the first launch after every upgrade start a second Nib beside the
  running one.
- **Declared, same as before:** a process running as the same user can read the record and answer the
  challenge. The record's permissions are the boundary (ADR-054).
- The nonce header is bounded only by the server's header limit.
- **Covered by** `TestAProbeIsAliveOnlyWhenTheAnswerProvesTheToken`,
  `TestAChallengedProbeBelievesTheInstanceThatHoldsTheToken`, `TestTheProbeRouteProvesItHoldsTheToken` and
  `TestAStaleRecordIsTakenOverWithoutUserAction`.
