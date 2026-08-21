# ADR-010 — An announcement carries the transport, not just the port

**Status:** accepted
**Date:** 2026-08-21
**Context:** ADR-007 (which this extends); `PLAN-signing-ceremony.md` D8, D14; law L1.
**Extends:** [ADR-007](007-discovery-announcement.md). ADR-007's identity reasoning —
the name and never the pin — is untouched and is not restated here.

## Decision

A link-local discovery announcement carries the **transport** its announced port
belongs to, as one wire byte, alongside the port. The format version moves from **1 to
2**, and a version-1 announcement is **refused**, not read.

The dialing side uses the **announced** transport for a discovered candidate. The
request's `transport` field still decides for an address the user **typed**, which is
the only case where there is no announcement to read.

## Why

**A port without its transport is not an address.** The armed side listens on exactly
one transport, chosen at arm time. Under QUIC the announced number is a **UDP** port;
under TCP it is a **TCP** port. The number looks identical and names two different
sockets.

Version 1 carried the number alone, so `dialPeer` chose a transport from the
initiator's own request. Arm with `{"transport":"quic"}` and no bind, initiate with no
address and no transport, and Nib sent a TCP dial at a UDP port — a connection refused,
surfaced to the user as *"that peer is not reachable"*.

## Why it was invisible

Both reasons are shapes that hide other defects, so they are recorded rather than
summarised:

- **The web client never sends a transport at all**, so the GUI was TCP on both sides.
  The two halves agreed by never disagreeing.
- **`build/pairrepro.sh` passed `-F transport=` to BOTH sides**, out of band. Tier 4 —
  the harness whose entire purpose is two real binaries completing a ceremony — was
  *configured past* the bug it would otherwise have been the thing to find. Two
  harnesses agreeing because something outside the protocol told them the same answer
  is not the protocol carrying it.

The harness is fixed in the same change: the LAN runs pass the transport to the armed
side only, and there are now **two** of them, TCP and QUIC, so the announced-transport
path is driven end to end by a real binary.

## Why a byte, and why the version bump

An enumeration, so the parser's whole job is a range check. A string would let an
unauthenticated datagram choose how many bytes this package allocates and how they are
compared, for a field with two legal values. An undefined value is `ErrMalformed`;
defaulting it to TCP would be the same guess this decision removes, made on a byte an
attacker picked.

The version is bumped and version 1 **refused** rather than best-guessed, because a
version-1 announcement's port could be either transport — reading one means guessing.
Nib has no users and forbids compatibility shims (`CLAUDE.md`), so there is no
version-1 speaker to keep working.

## Why the wire encoding is not `internal/p2p`'s constants

`TestNothingHereCanReachAnIdentity` forbids `internal/discovery` importing `p2p`,
`vault` or `sign` — that guard is what makes L1 structural rather than remembered. So
the wire owns a byte, `p2p` owns the transport names, and `internal/server` — the one
layer holding both — owns the mapping in each direction. A shared constant would be a
shared import, and the guard is worth more than the duplication costs.

`internal/server`'s own `transportTCP`/`transportQUIC` are now **aliases** of
`p2p`'s rather than second copies (ADR-009): the string that selects a dialer, the
string a listener reports and the string compared against a request are one value.

## What this does NOT change

**L1 is untouched.** The transport is reachability, exactly like the port and the
source address. It has no bearing on which peer is accepted: a lying announcer sends us
at a socket that does not answer as the pinned peer, and the handshake is what refuses
it. An announcement that lies still costs a browse and can never substitute a signer.

The listener is **asked** for its transport (`p2p.Listener.Transport()`) rather than
told alongside it. A port and a transport carried as two separate values are two facts
that can disagree, and the listener is the only thing that knows which socket it
actually opened.

## Consequences

- A ninth thing is on the link about an armed Nib. It is two bits of information about
  a socket that is already probeable by anyone who can see the port, so it adds nothing
  an observer could not measure.
- The announcement grows by one byte, against a 256-byte cap.
- A future third transport is a new enumerant, not a new field.
