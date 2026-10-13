# ADR-149 — a stranger holds a share of the handshake pool, and a mapped address is published only where the probe saw us

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-130 (what a stranger on the link can do to the rendezvous) to the two residues `/pending 775` left
open. ADR-061's counting (distinct source prefixes, a strict majority) is reused, not restated. ADR-011 is untouched:
nothing here sends a packet, or sends one earlier. ADR-010 is untouched: no record format moves.
**Context:** `/pending 775`, the P01 phase-close review's R7. Two things a host on the link could do, each read at
the line and one measured.

*The TCP handshake pool.* `tlsListener.acceptLoop` takes a slot from a sixteen-slot pool before it starts a
handshake, and the acquire blocks. A host that opens connections and says nothing holds a slot each for the 30 s
handshake timeout, and the pool refills as fast as it drains, so the genuine peer waits behind everything queued
ahead of it. Measured on this tree (loopback, one stalling source, the pinned peer dialling from another): with 16
silent connections open the pinned peer's handshake completed after 30.0 s, with 32 after 60.1 s, with 48 after
90.0 s — a handshake timeout per sixteen, as the pool's own comment had reasoned. The same three runs with this
decision in force: under 0.1 s each.

*The mapped address.* On the UPnP path the "router" is whatever answered an SSDP multicast, and the external
address is that responder's answer to `GetExternalIPAddress`. `screenedMappedEndpoint` refuses a private, shared
or low-port answer and passes any public one — and the address then goes into a signed candidate record that both
parties dial and punch at. A host on the link could so name a third party's address and have this machine and its
peer send traffic there.

**Applies:** `internal/p2p` (the TCP listener), `internal/rendezvous` (`SelfAddress.MajorityIP`),
`internal/server` (`appendMappedCandidate`).

## Decision

**1. One source holds at most four of the TCP handshake pool's sixteen slots; a connection past that is closed,
not queued** (`maxHandshakesPerSource`, `listenerCore.admit`). The count is taken before the pool is asked and
given back when the handshake goroutine ends.

**2. A source is an IPv4 address, or an IPv6 /64** (`handshakeSource`). A host owns the whole of a /64, so
per-address counting would hand it 2^64 sources.

**3. Loopback is exempt, by name.** Every multi-process tier and every test dials from 127.0.0.1, where nine
parties share one address; a process that can dial loopback is already on the machine.

**4. The QUIC listener has no such cap, by name.** A connection its `Accept` returns has completed the pinned
mutual handshake, so only the pinned peer reaches that pool; the TCP pool is the one a stranger's bare connection
lands in.

**5. A port-mapped address is published only when it is the address the DHT self-address probe saw this machine
at** (`corroboratedMappedEndpoint`, after the existing screen). "Saw" is `SelfAddress.MajorityIP`: the IP — the
port is ignored — that a strict majority of the probe's distinct source prefixes reported, with at least two
sources. The IP alone because the mapped port matters most behind an endpoint-dependent NAT, where no
address-and-port majority exists and `Class.Addr` is empty by design.

**6. With no probe answer for the family, the mapped address is not published** (fails closed), and the mapping is
deleted from the router as a screened-out one already is. Where the probe placed the machine at a *different*
address, the diagnosis is the screened case's ("another NAT beyond this router"), not "enable port mapping".

## Consequences

- A single stalling source no longer delays the genuine peer at all: twelve slots stay free of it.
- **Declared, not closed:** a host with four source addresses (four IPv4 addresses on the link, or four IPv6 /64s)
  still fills the pool, and the genuine peer then waits as before — the arm window ends that, as it ended the whole
  attack until now. Two parties behind one NAT are one source: a stranger there can hold the four slots the genuine
  peer would have used, and the peer's fifth concurrent connection is closed rather than queued.
- Four, not fewer, because the genuine peer is a source too: its racing dialer opens one connection per candidate
  address and a re-race can overlap the attempt it replaces. A refused connection is a lost candidate.
- A mapped address is no longer published on a machine whose probe got fewer than two answers. The reflexive
  candidates are missing there for the same reason, and the LAN tier and a typed address do not pass through this.
- A mapping behind a second NAT with a public address in between — which no one outside could dial — is no longer
  published either.
- Nothing a peer on an older build reads has changed shape: a record carries the same fields and, in the refused
  cases, one candidate fewer. An older dialer opens one connection per candidate and is far below the cap.
- **Refused:** queueing a source's excess behind its own share (the excess is the attack; it has no claim on a
  queue); a per-source cap on QUIC (decision 4); trusting one DHT node that agrees with the router (one node
  agreeing is ADR-061's "not corroboration", and may be the same host); cross-checking against `Class.Addr`
  (empty in exactly the case the mapping exists for).
- Not run here: tiers 4, 4 `-n 4` and 6 (fixed ports). They dial from loopback, which decision 3 exempts, and
  publish no mapped candidate; `build/dhtlive.sh` is the harness for the probe against real nodes.
