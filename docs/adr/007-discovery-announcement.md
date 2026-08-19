# ADR-007 — A discovery announcement carries the name, never the pin

**Status:** accepted
**Date:** 2026-08-19
**Context:** `PLAN-signing-ceremony.md` P03.S01; law L1; D3; caveat 3.

## Decision

A link-local discovery announcement (`internal/discovery`) carries exactly three
things: the sender's **six-word pairing name**, the **port** its armed session is
listening on, and a **per-process nonce**. It carries no fingerprint, no address, and
nothing else. Its socket is treated as internet-facing on every platform.

## Why the name and not the fingerprint

L1: *"the rendezvous path affects reachability only, never identity."* Acceptance is
the pinned fingerprint at the TLS handshake plus the spoken verification string, and
neither is reachable from the discovery package.

The name is enough because of the direction the pairing package permits. `Matches(fp,
name)` recomputes the name from a fingerprint **the receiver already holds**; there is
no name-to-fingerprint direction, because `decode` is unexported on purpose — *"a
half-fingerprint that looks like an identity is the misuse this design removed."*

So a browser can only recognise peers it has already pinned, and **discovery cannot
serve first contact at all**. Two strangers still need an invitation (D21). That is a
consequence worth stating plainly rather than discovering later: it is not a
limitation to be lifted, it is L1 holding structurally instead of by convention.

Announcing the name **string** rather than the 66 bits it encodes follows from the
same boundary: the string is what the exported comparison consumes, and carrying the
bits would require either exporting `decode` or duplicating its inverse.

## Why a nonce, and not the source address

`IP_MULTICAST_LOOP` defaults on, so a sender hears its own announcement. That stays,
for two reasons: it is what lets two instances on one host discover each other, and it
is **the only firewall-independent liveness check on the send path** — a browse that
hears neither a peer nor itself can say which of the two failed, and one that hears
itself has proved the send path works.

But it means every browser must discard its own announcements, and the source address
is the wrong discriminator: on a multi-homed machine the copy arrives from whichever
interface address was used, so recognising it means enumerating every local address and
re-checking whenever they change. A nonce minted once per process is exact.

## Why the socket is treated as hostile

Go binds a multicast listener to the **wildcard**, not to the group: `net`'s
`sock_posix.go` rewrites a multicast bind address to `0.0.0.0`, deliberately. Both
`net.ListenMulticastUDP` and `net.ListenPacket`+`JoinGroup` therefore accept ordinary
**unicast** on that port from any host that can route to this one — measured, not
inferred.

Binding the group address does refuse unicast, but only on Linux and only through a
raw-socket construct behind a `//go:build` tag — which is exactly the shape that has
already produced one silent defect in this repo (`ReplaceOthers` returning 0 off
Linux). So bind-scoping is not the boundary. The parser is: a fixed header read before
anything is allocated on the content, a hard 256-byte cap, exact length agreement, no
state created from an unauthenticated datagram, and the **zero** announcement returned
on every refusal path — the first draft returned what it had parsed so far, so a caller
who ignored the error would have acted on an attacker-chosen port and nonce.

## Why the version field is refused rather than best-guessed

This is a wire format other Nibs parse. A version field that is read and then ignored
is decoration. An announcement of an unknown version returns `ErrVersion`, distinct
from `ErrMalformed`, so a diagnostic can say *"there is a Nib here and we cannot talk
to it"* — a different message from *"something on this link is broken"*, and the
distinction is the whole value of having the field.

## Consequences

- Discovery locates peers you have already pinned. It cannot introduce one, and no
  future slice may make it able to without amending L1.
- `ErrNotOurs` is returned bare, so rejecting foreign traffic on a shared group
  allocates nothing — otherwise anyone on the link chooses this process's allocation
  rate.
- Interface filtering cannot rely on the arrival interface: `x/net`'s
  `SetControlMessage` is unimplemented on Windows (`control_windows.go` is a `TODO`
  returning `errNotImplemented`), and `payload_nocmsg.go` returns a nil control message
  with a nil error. Any filter written as `if cm != nil && cm.IfIndex != want` silently
  accepts everything there. Filtering belongs in the payload.

## Alternatives refused

- **Announce the fingerprint.** L1 permits it — a lying announcer still cannot
  substitute a signer — but it puts a stable identity on the wire for no gain, since
  the receiver must already hold that fingerprint for the announcement to be useful.
- **Share `224.0.0.251:5353` with mDNS.** It is the port a default-deny desktop already
  permits, and that is a real cost to pay elsewhere. Refused because it puts non-mDNS
  bytes on a group other implementations parse as mDNS; doing it honestly means
  speaking DNS-SD, which is a phase of its own, not a slice of this one.
