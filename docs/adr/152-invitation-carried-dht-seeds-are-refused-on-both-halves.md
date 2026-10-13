# ADR-152 — invitation-carried DHT seeds are refused on both halves

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-011 (the local link gets its window before anything leaves the machine), ADR-130 (a node is
admitted to the DHT table only for answering us), ADR-149 (a mapped address is published only where the probe saw
us). ADR-009 (one door, and its guard checks the door). Closes `PLAN-signing-ceremony.md` D6's second half as
built at P04.S06.
**Context:** `/pending 357`, decided by Dan 2026-10-12. D6's second half — bootstrap addresses carried in the
invitation, so a machine whose shipped list has rotted can still reach the DHT — was built at P04.S06 (v1.115.0)
and wired to nothing: no door that issues an invitation set `Invitation.Seeds`, and `rendezvous.Server.Seed` had
one caller, the `nib rendezvous` diagnostic. The item asked whether that was sequencing or a gap. It is now a
decision.
**Applies:** `internal/server` (every door that issues or parses an invitation), `internal/rendezvous`
(`seeds.go`, `Seed`, `SeedSample`), `internal/ceremony` (`Invitation.Seeds`, `validateSeeds`), `internal/cli`
(`nib rendezvous`), `build/dhtlive.sh`.

## Decision

**1. No invitation Nib issues carries seeds, and nothing feeds a parsed invitation's seeds to the DHT.** Both
halves, because either alone is useless or dangerous:

- *Consumed*, a seed list is where this machine's first DHT packets go, chosen by whoever wrote the invitation
  and never signed (a `Record` has no seeds to compare against). That is the chain P04.S06 wrote out — hostile
  seeds, a routing table holding nothing else, every `probeTargets` entry attacker-chosen, `classify`'s and
  `MajorityIP`'s majority satisfied, the punch budget aimed from this user's address — and, short of that, a read
  receipt: opening an invitation tells its sender's chosen addresses this IP and this moment. "Only after a
  demonstrated bootstrap failure" (`invSeedsOnly`) bounds how often, not whether.
- *Produced alone*, the addresses travel, are parsed and are dropped.

**2. The failure seeds were built to survive is answered at the shipped list.** `shippedSeeds`
(`internal/rendezvous/seeds.go`) records, for each literal, the name it was taken from, who operates it and
whether it answered at the last measurement. Measured 2026-10-12 (three attempts, `ping` and Nib's own
`find_node`): four of six answer — two addresses of Transmission's router and libtorrent's over IPv4 and IPv6 —
and each of the four alone bootstrapped a cold table. The two that are silent are BitTorrent Inc.'s. Nine other
well-known router names were measured and none qualified (they do not resolve, are silent, refuse the port, or answer
with no nodes), so nothing was replaced; the table in `seeds.go` is the record.

**3. Two answering operators over IPv4 is the floor.** The probe's majority counts source prefixes, not
operators, so it does not notice a table one operator filled. `TestTheShippedSeedsSpanTwoAnsweringOperators`
refuses a list recorded below that, with a duplicate, a non-public address or an unnamed operator. It reads the
record, not the network.

**4. The record is checked against the network on every `./build/dhtlive.sh` run.**
`TestLiveShippedSeedsAnswerAsRecorded` asks each address in turn and prints a `SEED` line — `as recorded`, `ROT`
or `BACK` — and the harness repeats the count. It fails on nothing a stranger controls. A cold start succeeds
while one address answers, so before this a mostly-dead list was invisible until measured by hand.

**5. The wire format does not change, and a received seed list is still parsed.** `Invitation.Seeds` stays
`omitempty` at the same `InvitationVersion`; `MaxSeeds` and `validateSeeds` stay at both doors. Parsed rather
than ignored for three reasons: an over-long or malformed list is still refused whole instead of being carried
unexamined; an invitation written by a build that sets seeds (the diagnostic, or any later one) reads the same
on every build; and removing the field would change nothing on the wire while deleting the validator the
diagnostic exercises. After parsing, the server reads the field nowhere.

**6. `nib rendezvous` keeps its loop** — `SeedSample`, the round trip through `Encode` and `ParseInvitation`,
`Seed`, the seeds-only retry and its separate counters. It samples its own table and feeds itself, so no
stranger chooses anything, and it is the one caller that keeps the validator, the attribution
(`InvitationSeedsTried` / `Used`) and the unsaved-cache rule exercised. `build/dhtlive.sh`'s `NIB_LIVE_SEEDS`
drives the same path with addresses the operator supplies.

**7. One guard holds 1 and 6.** `TestInvitationSeedsReachNothingButTheDiagnostic` (root package) fails on any
non-test file outside `internal/cli/rendezvous.go`, `internal/rendezvous` and `internal/ceremony` that calls
`Seed` or `SeedSample` or names a `Seeds` field.

## Consequences

- A machine with an empty or dead cache on a network where every shipped address is silent cannot reach the DHT,
  and no invitation can rescue it. A ceremony the local link answers (ADR-011) needs no DHT. This is the cost of
  the decision, accepted.
- The convene door, the re-issue door and `convenerInvitationFor` say "refused, ADR-152" where they said the
  question was open.
- **Declared:** an invitation is stored as the text that arrived, so a seed list a sender put in one sits in the
  vault with it. Nothing reads it.
- **Declared:** over IPv6 one operator answers. `DefaultWant` (v6 nodes learned through a v4 contact) is what
  covers a dual-stack host; a v6-only host starts from libtorrent alone.
- **Declared:** the two silent entries stay listed. A silent entry measured no delay on a cold start (three runs
  each way), and the names still resolve to these literals; whoever comes to hold a silent address would receive
  one datagram per cold start and could answer it. Removing them is a list edit that needs no ADR.
- Reversing this needs the recipient-side disclosure the item named (what opening an invitation contacts, before
  it is opened) and an answer to the eclipse chain that does not rest on the majority rule. It is a new ADR, not
  an edit to this one.
