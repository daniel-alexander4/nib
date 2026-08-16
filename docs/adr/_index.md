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

And the plan those decisions live in is temporary. `PLAN.md` retires once its
build order is walked (`STANDARDS.md` §15.6) — so without this directory the
reasoning would go with it, leaving a law in the codebase with no surviving record
of what it protects against.

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
