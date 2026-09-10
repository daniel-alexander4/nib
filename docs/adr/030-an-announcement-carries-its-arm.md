# ADR-030 — An announcement carries which ARM its port belongs to

**Status:** accepted
**Date:** 2026-09-09
**Context:** `/pending 420`. The format has been version **3** in the code since the hop
field landed; ADR-007 and ADR-010 both still read as though it were 2, and no ADR carried
the bump. ADR-010 is the decision that established a version bump is ADR-worthy, which is
what makes its own successor going unrecorded the defect this closes.
**Extends:** [ADR-010](010-announcement-carries-transport.md). Its identity and transport
reasoning is untouched and is not restated here.
**Applies:** anything that parses or emits a link-local announcement.

## Decision

A link-local announcement carries a **hop** field, and the format version is **3**. A
version-2 announcement is refused rather than best-guessed, exactly as version 1 is.

## Why

ADR-010's argument, one level in. That ADR's finding was that a port alone is not an
address, because the same number means different things under different transports and
guessing between them dialled a QUIC-armed peer over TCP.

The same shape reappeared one layer down. **A version-2 announcement's port could be
either ARM** — a hop arm or a delivery arm, on one machine, pinned to the same peer — and
the number is identical. Guessing between them is the defect the version field was added
to remove, so the announcement says which.

## Why refusal, and not a best guess

Unchanged from ADR-010 and restated only because it is the half a reader reaches for: a
version field that is read and then ignored is not a version field. Nib has no users and
forbids compatibility shims, so there is no version-2 speaker to keep working, and
accepting one would mean guessing the arm — which is the thing this version exists to
stop.

## Consequences

- ADR-007's *"exactly three things"* enumeration is now six fields
  (`version`, `transport`, `port`, `hop`, `nonce`, `name`); its identity reasoning stands.
- The offsets are named once (`offVersion` … `offNameLen`), which ADR-009's rule already
  required and which a prior bump had already cost: adding the transport byte moved the
  port, a mutation test went on patching the old offset, and a refusal test silently
  stopped testing its own refusal.
- A future field is a version **4** and an ADR, not a widened parse.
