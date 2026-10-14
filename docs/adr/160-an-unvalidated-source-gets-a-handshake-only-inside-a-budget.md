# ADR-160 — an unvalidated source gets a QUIC handshake only inside a budget

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-149 (one source holds a share of the handshake pool) to the transport it named as uncapped;
ADR-061 and ADR-130 (what a stranger may make this endpoint do). ADR-009 (one door).
**Context:** `/pending 766` part 4. ADR-149 bounded the TCP handshake pool and said of QUIC that "its pool is
reached only after the pinned handshake". That is true of Nib's pool and silent about the library's work: a QUIC
Initial costs its sender one datagram, and quic-go answers it with a connection — keys derived, the TLS server
flight computed (a key agreement and a signature), a reply sent, and the connection held for the handshake
timeout — all before the pinned-peer check, which is the client's certificate and arrives last. A UDP source
address is whatever the sender wrote, so that cost could be drawn by anyone, from nowhere, without receiving a
reply. `quic.Transport.VerifySourceAddress` was unset on all three transports.
**Applies:** `internal/p2p` (`sourcecheck.go`; the three `quic.Transport` literals in `quic.go` and `endpoint.go`).

## Decision

**1. An endpoint starts a handshake for an unvalidated source only inside a budget of 16, refilled at 4 a
second. Past it, the source must first prove it receives at the address it claims** — QUIC's Retry (RFC 9000
§8.1.2). `newSourceCheck` is the one door and every transport in the package installs it
(`TestEveryTransportChecksItsSources`).

**2. The budget is per endpoint, never per source.** The source is the thing not yet known: a budget keyed by
it is a budget whose keys the attacker picks.

**3. Past the budget an honest peer is asked, never refused.** A Retry is stateless on this side — one token
sealed, one datagram out, nothing held — and the client answers it by itself on the same socket, for one round
trip. So the budget needs no loopback exemption (ADR-149's TCP share has one, because that share REFUSES), and
no tier is configured past it.

**4. No per-source share over QUIC, declared.** A sender at a real address can answer the Retry and be given a
handshake that then fails the pin. That sender now has an address, which is what a share would be kept against;
it is not built here.

## Measured (loopback, two real endpoints, quic-go v0.61.0, one run)

Honest handshakes, 40 in sequence, deferred-stream path (`QUICListenHandshakeOn` / `QUICDialHandshakeOn`):

| every Initial… | completed | median | p90 |
|---|---|---|---|
| given a handshake (before) | 40 / 40 | 6.8 ms | 12.9 ms |
| sent a Retry first | 40 / 40 | 7.1 ms | 13.4 ms |
| this rule (16, then 4/s) | 40 / 40 | 5.9 ms | 11.0 ms |

The glare — both endpoints punching, listening and dialling each other at once on one socket each, 20 rounds:
20 / 20 with no Retry (mean 13.4 ms) and 20 / 20 with a Retry demanded of every Initial (mean 16.3 ms, 80
Retries answered). The whole `internal/p2p` test package passes with a Retry demanded of every Initial.

Unfinished handshakes — k dials from a client that never hears a reply, read 1.2 s in. The figures are for the
whole process, client included, so the server's share is the difference between rows:

| k | every Initial… | goroutines | heap in use | CPU per Initial | an honest dial meanwhile |
|---|---|---|---|---|---|
| 400 | given a handshake | +3,209 | +49.3 MB | 2.30 ms | 4.3 ms |
| 400 | sent a Retry | +1,609 | +19.4 MB | 0.99 ms | 14.5 ms |
| 400 | this rule | +1,688 | +19.4 MB | 1.18 ms | 3.2 ms |
| 1,600 | given a handshake | +9,958 | +121.0 MB | 1.52 ms | 10.6 ms |
| 1,600 | sent a Retry | +6,412 | +70.6 MB | 0.95 ms | 13.3 ms |
| 1,600 | this rule | +6,485 | +76.4 MB | 0.98 ms | 2.8 ms |

So an unvalidated Initial given a handshake held about four goroutines and 30-75 KB on this side for the
handshake timeout and cost about 1.3 ms of CPU; sent a Retry it holds nothing. (At 100 the same shape: +809
against +409 goroutines.) The spoofed-source case itself was not driven — crafting an Initial needs the
protocol's own packet protection — and is the same code path: a source that never answers.

## Consequences

- A flood from sources that cannot receive costs this endpoint a sealed token and a datagram each, and the
  connection-id draw a Retry makes is free since the router stopped remembering ids (`udpmux.NewConnectionID`).
- A peer that meets the budget pays one round trip. In a ceremony that needs more than sixteen unvalidated
  Initials inside four seconds at ONE endpoint; an arm is dialled by one pinned peer racing a handful of
  candidates, paced by `reraceWait`.
- A Retry token is bound to the source address quic-go saw. A client whose NAT mapping changes between its
  Initial and its answer fails that attempt and is re-raced; the punch uses one socket throughout, so its
  mapping does not move.
- **What tiers 4, 4 -n 4 and 6 must show:** green as before over QUIC, on the LAN and the DHT legs — the budget
  is not met by a nine-party relay's own traffic, and where it is the handshake still completes.
- Not measured: a real NAT, and a peer on a quic-go older than Retry support (none ships in any Nib build).
