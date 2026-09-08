# ADR-028 — A dial declares its role before either side picks a gate set

**Status:** accepted
**Date:** 2026-09-08
**Context:** `/pending 385`; ADR-010 (whose argument this repeats one layer in); ADR-009
(one door per rule); `PLAN-signing-ceremony.md` D24, L2.
**Extends:** [ADR-010](010-announcement-carries-transport.md). That ADR's identity and
transport reasoning is untouched and is not restated here.

## Decision

A session connection carries a one-byte **role** frame, written by the dialer and
acknowledged by the responder, **before** the spoken check and before either side has
chosen a gate set. Two roles: `RoleCoSign` (the ceremony hop exchange, served by
`Receive`) and `RoleTransfer` (the one-way document transfer, served by
`ReceiveDocument`).

The session ALPN moves from **`nib/2` to `nib/3`**, and the frame is sent **only** to a
peer that negotiated `nib/3`. Older versions stay in the offer list, so an older peer
negotiates down and gets this build's pre-role behaviour rather than a handshake failure.

**Zero is not a role.** `RoleCoSign` is 1; a zero byte is `roleUnset` and is refused by
name.

**The role exchange happens at the same level on both sides** — in `internal/server`,
which owns the connection and the gate sets — never inside `Initiate` / `Receive` /
`SendDocument` / `ReceiveDocument`.

**An arm's `mode` is its policy and the role is the request; the wire never overrides the
arm.** A listener armed for a transfer refuses a co-sign dial, by name. `armServesRole` is
the one door for that question.

**The delivery arm serves both roles**, and a co-sign reaching it gets the **real** human
Confirmer and Verifier — never the unattended gates.

## Why

**A connection that cannot say what it is for is not an address.** ADR-010 added a
transport byte because "a port without its transport is not an address", and announcement
v3 added the hop for the same reason. This is the same defect one layer in: which
responder role ran was decided by the **arm**, before the wire, and nothing was read off
the connection to choose between them.

That is fine while a machine holds one kind of arm, and it stops being fine the moment one
machine needs both. A party that has committed its contribution **has a record**, so the
hop sweep skips it (`ceremonyarm.go`: `if st.State == ceremony.LoadOK { continue }`) and
the delivery sweep arms it instead — and that arm auto-confirms and can never serve a
stored contribution, because `ReceiveDocument` does not reach `coSignExchange`. The
resumed hop met an arm that structurally could not answer it. Measured as a one-in-three
tier-4d failure on the interrupt clause.

**The party could not have chosen the right arm, and that is what settles the design.**
Nothing is written between `persistContribution` and the frame reaching the initiator, so a
party that died in that window is byte-identical on disk to one whose hop landed. Any
scheme that makes the party *guess* is guessing on a fact it does not hold. The dialer
knows. So the dialer says, and the guess is removed rather than resolved in someone's
favour.

**Why not restore the hop arm instead.** That was the smaller option and it is refused: it
requires the guess above, and it would make the two sweeps contend for one population —
destroying the disjointness `ceremonyarm.go` documents at length ("which ceremonies are
waiting is answered by the ABSENCE of a record, and that is exact"), with one interactive
slot meaning one of them must lose.

**Why a negotiated ALPN and not an unconditional frame.** `alpn2`'s own reasoning: a build
that predates this one, handed a role frame where it expects the verification exchange,
does not fail cleanly — it reads the byte as something else and produces a verdict about
its counterparty from a version skew, which D32 forbids. `SpeaksRoleFrame` is a **floor**
looked up in the offer list and fails closed, exactly as `SpeaksNamedRefusals` is; that
predicate's doc warned a third version would expose an equality, and `alpn3` **is** that
third version.

**Why an acknowledged round trip and not a fire-and-forget byte.** A responder that closed
on a role it does not serve would reach the initiator as a bare EOF — the class this repo
has now found at four sentinels, where a decision arrives looking like a dropped
connection. The ack costs one round trip with no human in it and buys a sentence; the
refusal carries wire code **16**, frozen.

**Why the exchange is not inside the exchange functions.** The first cut put the write in
`Initiate` / `SendDocument` and the read in the server, and any direct pairing then wrote a
frame nobody consumed. The read must be in the server — the delivery arm reads the role to
choose which exchange to run — so the write belongs there too. Symmetry at the same level
is the property; it was found by the p2p suite desynchronising, not by review.

**Why the real gates for a co-sign on the delivery arm.** `deliverOneLeg`'s header records
that the unattended gates are sound on a delivery leg **and only there** — the two parties
met at their hop and answered those words about that pin. A resumed hop *is* that hop being
taken for the first time, so it owes the human spoken check and the human consent.
`TestTheUnattendedGatesHaveOneDoor` keeps this structural rather than conventional: the
co-sign branch routes through `serveOneSession`, which is `p2p.Receive`'s one production
call site and constructs the real `Confirmer` and `Verifier`.

## Cost, stated rather than discovered

`DeliveryLegBudget` gains a fourth term, `RoleDeadline` (30 s): **14m → 14m30s** unattended,
**24m → 24m30s** interactive. At `MaxRoster` 32 that is ~16 minutes on a ceremony's reserved
`DeliveryBudget`. The term is reserved because the code arms it, not because it is large —
the budget's own guard records what the opposite reasoning cost last time, a leg short by 22
minutes.

## What this does not do

It does not make the two sweeps overlap, does not change which ceremonies either sweep
takes, and does not touch ADR-011's LAN-first hold. `/pending 380`'s question — whether the
delivery sweep should admit a pre-hop party at all — is untouched and is still open.
