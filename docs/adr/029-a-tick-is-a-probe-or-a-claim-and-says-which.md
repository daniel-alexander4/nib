# ADR-029 — A tick is a probe or a claim, and the row says which

**Status:** accepted
**Date:** 2026-09-09
**Context:** `/pending 417`, found at P03's phase-close review. The behaviour shipped
before this ADR was written, which is the defect this document also closes: a new
architectural decision gets an ADR **in the same change**, and this one did not.
**Supersedes:** ADR-027's *"A step is ticked only where Nib can observe it. `done` is a
probe or it is `null`."* The rest of ADR-027 — why the checklist exists at all, that it
is advice containing two one-way doors, and that the seven probed steps are probed —
stands untouched.
**Applies:** the signing checklist, and any later surface that mixes observed state with
a user's assertion about it.

## Decision

**Every step can be ticked by hand, including the ones Nib can already see — and the row
records which of the two it was.** `done` is now a probe **or** a person's claim, never
an unattributed tick.

- A probe that answers true renders as Nib's own observation.
- A step the user marks renders as *her* claim, labelled as one, and says so on hover.
- A step with neither stays `—` and says Nib cannot tell, exactly as ADR-027 required.

## Why ADR-027's rule was too strong

ADR-027's reasoning is right about the failure it names: *"a tick not backed by a signal
is worse than no tick, because the entire value of the list is answering what is left."*
That holds for an **unattributed** tick. It does not hold for one that names its author.

The rule as written also made the checklist unable to do the job it was built for. Eight
of its steps have no probe and never will — Nib cannot know whether you ran a
hidden-content scan, emailed the file, or where you put an `.ots`. Under ADR-027 those
eight were permanently `—`, so a user working through the list had no way to record the
thing the list exists to track: **what is left**. The list answered its own question only
for the steps that needed it least.

And a probe is not always right about the user's situation. A person whose judgement
differs from a probe's — a scan she ran outside Nib, a step she has decided does not
apply — could not say so.

## What keeps ADR-027's guarantee intact

The guarantee was that a reader can trust a tick. That is preserved by **provenance, not
by scarcity**: `row.dataset.by` is `nib` for an observation and `hand` for a claim, the
mark's hover text distinguishes them (*"You marked this done — click to clear"*), and a
manual tick never overwrites a probe's answer — `byHand && !seen` is what makes a row
read as a claim, so a probe that says true still reads as Nib's.

So the two never pretend to be the same evidence, which is what ADR-027 was protecting.

## Known limitation, stated rather than discovered

Manual ticks are **session-scoped and in memory**: not written to the vault, not
surviving a restart, and not per-document. Per-document progress has nowhere to live
today and inventing a store for it is a larger decision than this checkbox. Recorded here
so the next reader does not find out by testing it.

## Consequences

- ADR-027's headline rule is no longer the rule; its `docs/adr/_index.md` line and
  `web/index.html`'s copy of it are corrected in the same change as this ADR.
- A surface that later mixes observation with assertion inherits this: mixing is allowed,
  and the mixing is only safe while the surface says which is which.
