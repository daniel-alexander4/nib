# ADR-011 — The local link gets its window before anything leaves the machine

**Status:** accepted
**Date:** 2026-08-27
**Context:** P03's exit criterion (*"a LAN ceremony completes with NO outbound internet
traffic"*); `PLAN-signing-ceremony.md` D6, D8, C05; P07.S05d.
**Applies:** [ADR-009](009-one-door-per-rule.md). The one-door rule is the mechanism
here, not the decision, and is not restated.

## Decision

**Nothing contacts the public DHT until the local link has had its window, and where a
browse has already answered, the hold is on that answer rather than on a timer.** That
covers all three verbs, not only the two that are obviously network calls:

- the **bootstrap** — `Bootstrap` is called from exactly one function,
  `ceremonyID.ensureBootstrapped`, lazily, at first DHT use;
- the **fetch** — `feedCandidates` waits `browseWindow` before its first `Fetch`;
- the **publish** — `publishLoop` already took `browseWindow` as its first delay, and
  that stays.

The **hold** is `browseWindow` when nobody knows whether the link will answer, and
`lanFirstBudget` (30 s) when the browse already found the peer there — the dial side's
`peerAddresses` runs before the race, so a `sourceLAN` candidate is evidence, not a guess.

`bootstrapDone` — the flag gating D19's arm-side diagnosis — is set **inside**
`ensureBootstrapped`, at the moment the attempt completes, whether it succeeded or not.

## Why

**A local-first tool that phones home for a ceremony in one room is not local-first**,
and P03's exit criterion says so in the plan's own words. It was false.

Measured in the tier-4 namespace with an nft counter on off-link traffic: a **two-party**
LAN ceremony emitted **0** packets; a **four-party** LAN relay emitted **120**. The
difference is the invitation. Three sites bootstrapped eagerly — `dialerCeremony` at
construction, and **both** arm paths, which are different functions
(`startArmedRendezvous` for TCP, `runCeremonyReceive` for QUIC) — so the public DHT was
contacted on every hop of every ceremony carrying an invitation, which is every ceremony
P07 builds.

**It survived four phases because the only `--lan` run was the two-party one**, which has
no invitation and therefore no ceremony object at all. The run that existed to prove the
criterion was the one shape that could not reach the defect.

### Both halves, or neither

Deferring the bootstrap alone buys nothing. `publishLoop` had always waited the window
and said why; the **fetch** did not. A bootstrap deferred to first use, with an unwindowed
fetch immediately after it, moves the first off-link packet by microseconds. The two
changes are one decision and are recorded as one.

### A window is the wrong figure once the browse has answered

With the bootstrap lazy and both verbs behind a fixed two-second window, a nine-party LAN
relay **still emitted 105 packets**. A stack probe named `publishWhenSlow`: its gate was a
2 s timer racing the hop, and hops take 1–3 s, so the timer lost often enough to leak.
`browseWindow` is how long a browse *listens*; it was never a statement about how long a
link-local dial takes. Where the browse has already answered, the hold is
`lanFirstBudget` — an order of magnitude past the worst observed hop and a tenth of
`connectDeadline`. It is a hold and not a refusal: a stale LAN candidate still falls
through to the DHT.

### Why lazy rather than conditional

The obvious shape is *"bootstrap only when the browse returned nothing"*, and it works on
the dialer — except that `dialerCeremony` runs **before** `peerAddresses` browses, so the
answer is not in hand where the bootstrap was. It does not work on the arm at all: an arm
cannot know whether the dialer will find it on the link. Laziness needs neither answer.
The DHT is reached by whoever actually needs it, after the window, or never.

### The cost, measured rather than assumed

`TestTheAddedLatencyToTheDHTTierIsMeasured` reports **2.005 s** — one `browseWindow` — from
hop start to first DHT reach with no LAN peer. It is one-sided and bounded: a ceremony the
link answers pays nothing and emits nothing, and D8's ladder races the tiers concurrently,
so this is latency on **one rung** and not on the ceremony. `ceremonyHopBudget` is
unchanged and is now a looser bound than before, because the bootstrap no longer runs
serially ahead of the connect race.

## What it costs, and what it does not

`sync.Once` means one attempt per ceremony object. That is **exactly** what the three
eager calls already did — a fresh `ceremonyID` per hop on the dialer, one per arm — so it
preserves today's behaviour rather than introducing a limit on retry. Said out loud
because Once otherwise reads as one.

A bootstrap failure is still not fatal. The LAN and manual tiers are untouched, D19 cause
2 is the sentence for a dead DHT, and it is now reported by the verb that needed the
network rather than by an arm goroutine returning early.

## What this does NOT fix, and it is measured rather than left implied

**P03's exit criterion is still red at nine parties**, and this ADR does not claim it. Four
parties went 120 → 9; nine parties stayed at 60–111. Probed per instance: **i1 and i2 reach
the DHT zero times, and i3 through i9 reach it twice each** — from the arm's own `connect`,
not the dialer's. All eight signing parties arm before hop 1 and an arm's window is 2 s, so
from the third party onward every arm pre-publishes while waiting for a turn that has not
come.

The dial side's rule does not extend to it. An arm **announces rather than browses**, and
`answerLoop` filters sightings to its own expected peer, who is not on the link until the
hop begins — so the arm cannot tell "my peer arrives in fourteen seconds" from "nobody is
here". Giving it a signal that is evidence rather than another timer is **P07.S05e**.

Recorded here rather than in the plan alone because an ADR that stated the rule without
stating its reach would read, to the next author, as a criterion already met.

## Guards

- `TestTheDHTBootstrapHasExactlyOneDoor` — an AST scan: `Bootstrap` is called from
  `ensureBootstrapped` and nowhere else in `internal/server`. It asserts **routing**, so a
  fourth site added later fails whatever it looks like; a fourth site added without one is
  how this defect existed.
- `TestTheFeedDoesNotTouchTheDHTInsideTheLANWindow` — the fetch half.
- `TestTheBootstrapDoorSetsItsFlagEvenWhenTheBootstrapFAILS` — a flag set only on success
  inverts D19: the machine whose network is actually dead is the one that never gets told.
- `TestALANFoundPeerHoldsTheDHTPastTheBrowseWindow` — both arms. A hold that applied to
  every dial would not be a fix, it would be a DHT tier that never starts.
- `./build/pairrepro.sh --lan -n 9` — the criterion itself. **Still red**, for the narrower
  reason above; the harness now carries the per-instance distribution so a different one is
  not mistaken for the known remainder.
