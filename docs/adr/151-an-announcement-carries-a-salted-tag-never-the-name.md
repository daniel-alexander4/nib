# ADR-151 — an announcement carries a salted tag, never the name

**Status:** accepted
**Date:** 2026-10-12
**Supersedes:** ADR-007's **name field** only ("the sender's six-word pairing name", and its "why the name and
not the fingerprint" in so far as it argues for the name). ADR-007's other decisions stand unchanged: no
fingerprint and no address on the wire, the per-process nonce, the socket treated as internet-facing, a
version refused rather than best-guessed.
**Extends:** ADR-010 and ADR-030 (the format version is bumped and the previous one refused — 3 → **4**).
ADR-011 and ADR-134 are untouched: who announces, when, and for how long does not change.
**Context:** `/pending 3`, decided by Dan 2026-10-12. ADR-007 refused announcing the fingerprint because it
"puts a stable identity on the wire for no gain". The six-word name is a fixed function of that same
never-rotating fingerprint, so the reason applied unchanged to what was adopted — and P03 then changed who
receives it, from people the user chose to every device on every attached segment, every 500 ms, for up to
five minutes an arm.
**Applies:** `internal/pairing` (`tag.go`), `internal/discovery` (`announce.go`, `mcast.go`),
`internal/server` (`lan.go`, `discover.go`), `internal/cli` (`discover.go`).

## Decision

**1. The announcement carries a 16-byte salt and a 16-byte tag in place of the name.**
`tag = SHA-256("nib-discovery-tag-v1\0" ‖ salt ‖ fingerprint)[:16]`. Every input has a fixed length, so the
concatenation needs no length prefixes; the label keeps the hash apart from every other SHA-256 Nib takes
over a fingerprint. SHA-256 because it is the hash the fingerprint, the verification string and the
invitation derivations already use, and no key exists to make an HMAC mean more than this does.

**2. A browser recognises an announcer by recomputing the tag over each of its own pins**
(`pairing.TagMatches`), exactly where `resolve` called `pairing.Matches` per pin. Nothing leads from a salt
and a tag back to a fingerprint, so L1 holds as it did: discovery locates peers already pinned and cannot
introduce one. `internal/discovery` still never sees a fingerprint — salt and tag are opaque bytes to it and
`TestNothingHereCanReachAnIdentity` is unchanged.

**3. A salt belongs to ONE announcer and is drawn by ONE door.** `pairing.NewTag(fp)` draws it from
`crypto/rand` and takes no salt, so none can be reused or derived from anything stable; the server calls it
once per `startAnnouncing` (through `announcementOf`). "Per arm" therefore means per announcer: the arm's
own five-minute announcement, each answer to a seeker and the dialling side's hop burst each have their own.
An all-zero salt is refused by the encoder AND the parser — it is what an announcer that skipped the door
would send, and every such announcer would send the same one.

**4. The format is version 4, one fixed length (50 bytes), and version 3 is refused.** The version is read
before the length, because a version-3 datagram is 42–72 bytes and must be refused as *another version*
(`ErrVersion`), never as malformed or foreign. Reading version 3 was refused rather than kept alongside: a Nib
that still recognises names keeps every peer needing to send one.

**5. Nothing shows an announced name, because there is none.** A matched candidate's `Name`
(`GET /api/lan/heard`, the harness's printout) is the PIN's six words, computed on the receiving side — the
same words as before. The hop-mismatch log line, which runs before the pin scan, names the datagram's source
address. `nib discover`, which holds no pins, prints "a Nib on port N (transport) from ADDRESS".

**6. Another version's announcement is counted as that.** `discovery.Stats.OtherVersion`, apart from
`Malformed`, printed by `nib discover` with the sentence "both must run the same version".

## What this does NOT hide

**Anyone who holds a fingerprint can test a tag against it, exactly as a pin holder does** — that is the
mechanism, and a fingerprint is in every document its owner signed. So a party holding a document Alice
signed can still passively confirm "Alice is armed on this link now", on any network they can listen on.
The salt does not and cannot remove that without a secret shared between the two parties, which a
first LAN meeting does not have.

What it removes is everything that needed **no** fingerprint: a listener who does not already hold Alice's
cannot tell that two arms — two ceremonies, two days, two networks — are one party, cannot build a list of
"the identities seen here", and cannot learn six words to recognise later or look for beside a signature.
A recorded salt and tag can be replayed, as a name could be claimed; it buys what it always bought — a dial
the TLS pin refuses (L1).

## Measured

- **Cost of recognition** — `resolve` against an announcement that matches no pin (the whole list is
  scanned), 2,000 rounds: **1 µs at 1 pin, 13 µs at 32, 420 µs at 1,000**, no allocation. The six-word
  comparison it replaces measured 2 µs, 106 µs and 2.0 ms on the same machine. The only reader that lives
  longer than a two-second browse (`answerLoop`) holds exactly one pin.
- **Truncation** — 128 bits. A chance match against another party's pin costs one refused dial; the name it
  replaces carried 66 bits. Nothing here is a commitment an adversary gains by colliding.
- **Size** — 50 bytes fixed, against 42–72 for version 3.

## Consequences

- **A Nib on version 3 and one on version 4 do not find each other on a link, in either direction**, and
  each falls through to the next tier of the ladder (a typed address, or the DHT for a ceremony). The newer
  one can say so (`OtherVersion`); the older one counts it as malformed. The window does not yet say it —
  the network test's verdict still reads "no other Nib is announcing here" (residue, `/pending`).
- Rate limits and deduplication were never keyed on the name (`hopMismatches` is process-wide; a browse
  dedupes on pinned fingerprint + address), so none moved.
- `pairing.Matches` has no running caller left — the browse was its only one. It is kept and declared
  test-only (`zerocaller_test.go`): the name is still shown to people, and the encoding's properties are
  asserted through it.

## Alternatives refused

- **Keep the name and shorten the announce window.** Fewer datagrams of the same constant; one is enough to
  link.
- **An HMAC keyed by something the two parties share.** There is no such key at a first LAN meeting — the
  pin is public material — and a per-pair key would make an arm announce once per expected peer, telling
  the link how many it expects.
- **Reuse the per-process nonce as the salt.** One field, two jobs: the nonce is also every listen-only
  socket's self-filter and is 64 bits. A salt with its own door is the property; sharing a field is how a
  later change to one job breaks the other.
- **Rotate the salt inside one announcer.** The datagrams of one announcer share a source address and a
  port; rotating the salt hides nothing between them and costs a browse its repeat-recognition.
