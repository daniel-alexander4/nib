# ADR-134 — the window's arm names QUIC exactly when it carries an invitation

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-010 (an announcement carries the transport) and ADR-113 (the invitation field goes with the
ceremonies switch).
**Context:** /pending 833. `armRecv` (`web/app.js`) sent no `transport`; the server makes an absent field TCP, and
a ceremony armed over TCP publishes nothing to the rendezvous — while the form tells the user that with an
invitation Nib finds the other person over the public DHT. QUIC was reached only through the arms the machine
raises itself (the accept-time sweep, `armForDelivery`), so the manual arm was the one ceremony arm that could not
be found.
**Applies:** `web/app.js` `armRecv`; two comments in `internal/server/session.go`.

## Decision

`armRecv` sends `transport: "quic"` exactly when it sends an invitation, and nothing otherwise: a plain LAN arm
stays TCP. There is still no transport control in the interface — the transport follows from what the arm is for.

## Consequences

- The manual ceremony arm matches the accept-time arm it stands in for.
- A typed "Listen on" address with an invitation is then a UDP port; the form's help text does not say so yet.
- The root request-field census no longer lists `transport` as harness-only.
