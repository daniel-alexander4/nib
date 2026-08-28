# Architecture Decision Records

Short documents capturing significant design decisions and their rationale: what
was decided, why, and what was considered instead.

**ADRs are immutable in their decision content.** If a decision is reversed, write
a new ADR that supersedes the old one rather than editing it. A terminology
refresh may be applied in place with an `**Updated YYYY-MM-DD:**` note in the
header saying what changed; the decision and its rationale stay frozen.

## Why this directory exists, and why it starts at two

Nib went a long way without ADRs, and that was the right call while its
architecture was "one binary, one document, pdf.js in the middle" — a shape you
can read off the code in an afternoon. `STANDARDS.md` §11 puts it as *worth
adopting once a project has real architecture; overkill for tiny apps.*

Two things changed that. The multiple-open-documents work introduces a **repo
law** (ADR-001) that constrains every future async operation whether or not its
author has read the plan, and a **client architecture** (ADR-002) whose rationale
is a set of empirical findings about pdf.js and the DOM that are not recoverable
from reading the resulting code.

And the plan those decisions lived in was temporary. `PLAN.md` retired once its
build order was walked (`STANDARDS.md` §15.6) — so without this directory the
reasoning would have gone with it, leaving a law in the codebase with no surviving
record of what it protects against.

**That retirement happened on 2026-08-19** (all seven phases closed 2026-08-17 at
v1.108.4). Before the file was removed it was audited for reasoning that existed
nowhere else, and three decisions were written up here as ADR-004, ADR-005 and
ADR-006 — the wire protocol for document identity, the open-document cap's measured
byte figure, and the hand-off credential's security posture including the stronger
mechanism that was refused. Code comments that cited `PLAN.md` for a measurement or
a pin now cite the ADR that carries it. The file itself remains in git history; what
is gone is the working document, which is what retirement means.

A dated note inside ADR-002 still refers to `PLAN.md` in the present tense. That is
left as written: an ADR's text is the record of what was known when it was written,
and that note is superseded two paragraphs later within the same document.

**Earlier decisions are deliberately not backfilled.** Loopback-only binding, the
SSH-key-sealed vault, the single embedded binary, client-side fill through pdf.js
— all real architecture, all already documented where they are enforced (in
`CLAUDE.md`'s rationale-carrying rules, in code comments at the guards
themselves). Writing them up retroactively is a separate exercise worth doing on
its own merits, not a prerequisite for recording the two decisions that needed a
home today.

## Decisions

- [ADR-001: Operation pinning](001-operation-pinning.md) — no operation acts on a
  document it did not capture at its start; ids are never reused
- [ADR-002: One PDFViewer per document](002-per-view-viewers.md) — hidden, never
  destroyed, because an overlay's value lives in the DOM
- [ADR-003: Global history budget](003-global-history-budget.md) — the undo/redo
  byte budget is one figure for all open documents and bounds the undo+redo pair
- [ADR-004: Document id on the wire](004-document-id-on-the-wire.md) — an
  `X-Nib-Doc` header, optional only for the CLI, with a per-process epoch and 409
  for a document the server no longer holds
- [ADR-005: Open-document cap](005-open-document-cap.md) — count **and** aggregate
  bytes, refusing on whichever binds first, with the byte figure's method
- [ADR-006: Hand-off credential](006-handoff-credential.md) — a separate on-disk
  secret authorising one route, and why kernel-vouched peer credentials were refused
- [ADR-007: Discovery announcement](007-discovery-announcement.md) — the name, a
  port and a nonce; never the pin, and the socket treated as hostile everywhere
- [ADR-008: The byte cap binds every growth door](008-the-byte-cap-binds-every-growth-door.md) —
  extends ADR-005: the byte half was enforced only at open, so five writers of `doc.data`
  went past it
- [ADR-009: One door per rule](009-one-door-per-rule.md) — a rule that has to hold at more
  than one call site is written once and its guard checks the door, not the text; with the
  six from one review that reached some sites and not others
- [ADR-010: An announcement carries the transport](010-announcement-carries-the-transport.md) —
  extends ADR-007: a port without its transport is not an address, so a QUIC-armed peer was
  dialled over TCP; format version 2, and the tier-4 harness that was configured past it
- [ADR-011: The link gets its window first](011-the-link-gets-its-window-first.md) — nothing
  reaches the public DHT until the local link has had `browseWindow`: the bootstrap is lazy
  behind one door, the fetch waits as the publish always did, and the dial side holds on its
  browse result rather than a timer, and the ARM holds on evidence — a sighting of its own expected
  peer, which `answerLoop` already resolved. 120 off-link packets → 9 → 0; the two-party run that
  was supposed to prove the criterion was the one shape that could not reach the defect
