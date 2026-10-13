# ADR-165 — a QUIC connection id proves itself, and no table remembers it

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-061 (one UDP socket shared by the DHT and QUIC), ADR-149 / ADR-160 (what a stranger may make this
machine do before the pinned handshake).
**Context:** `/pending 669` #2 and `/pending 691` R7-3. The shared socket's demultiplexer (`internal/udpmux`) kept
a table of the connection ids quic-go had issued, each with a five-minute lifetime, and routed a short-header
datagram to quic-go only when its id was in the table. Two defects came from the table itself. A transfer resumed
after a pause longer than the lifetime rotated to an id that had been issued and never used, the table had
forgotten it, and the datagrams went to the DHT: reproduced over real quic-go, a 40 MiB write after a six-minute
pause stalled at about 13,000 packets with 270 datagrams misrouted. And every QUIC Initial from anyone registered
an id for five minutes before any authentication, swept quadratically. No lifetime can be right, because quic-go
reports neither a retired id nor a closed connection — its `ConnectionIDGenerator` is two methods.
**Applies:** `internal/udpmux` (`NewConnectionID`, `knownCID`), `internal/p2p/quic.go` (the generator).

## Decision

**1. An id carries its own proof.** A connection id is 8 random bytes followed by a 64-bit keyed tag over them
(`hash/maphash`, a seed drawn per process). `knownCID` recomputes the tag from the datagram and stores nothing.
Ids are 16 bytes.

**2. There is no table, no lifetime and no sweep.** An id issued a month ago is still this process's; an id this
process did not issue is not, whenever it arrives.

**3. The tag is a discriminator, not an authenticator.** A forged tag only routes a datagram to quic-go, which a
long-header bit already does for anyone; quic-go then discards what belongs to no connection. Do not treat a
passing tag as evidence about the sender.

## Consequences

Per datagram: about 8 ns and no allocation, against about 90 ns for the table lookup. A restart draws a new seed,
so ids from before it are foreign — as every connection from before it is gone. `RegisterConnectionID` and the
refresh test are removed. Held by `TestAnIssuedConnectionIDIsOursHoweverLongAgoItWasIssued` and
`TestATransferAfterALongPauseCrossesItsConnectionIDRotations` (real quic-go, a real pause).

**Declared:** the paused-transfer test was not re-run under a mutation (it would wait out a sixty-second stall);
the two mutations shown red are on `knownCID` itself.
