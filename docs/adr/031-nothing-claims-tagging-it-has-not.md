# ADR-031 — Nothing claims tagging it has not, and every operation declares its tag fate

**Status:** accepted
**Date:** 2026-09-11
**Context:** `PLAN-accessibility.md` laws 1 and 2, built at P01.S01–S04 (`/pending 29`).
**Applies:** every operation in `internal/pdfops` that takes a document and returns one.

## Decision

**Law 1 — nothing claims tagging it has not.** No output may carry `/MarkInfo /Marked true`,
a `/StructTreeRoot`, or a conformance assertion over content that is neither tagged nor
marked as an artifact.

**Law 2 — every operation declares its tag fate.** One verdict per operation —
`dropped`, `carried` or `untouched` — in one table, and a guard asserts that every
operation *has* one, enumerating the population from the code.

## Why a false claim is worse than a visible loss

A screen reader told a document is tagged **stops reaching for the fallbacks it would
otherwise use**. A user who can see their tagging is gone can re-tag, re-export, or choose
a different tool; a user handed a document that asserts structure it does not have has had
their own check defeated. That asymmetry is the whole of law 1, and it is why the remedy
is dropping the claim rather than preserving it — honesty is reachable today and
preservation is not.

## Why this was not hypothetical

Measured 2026-09-11 against pdfcpu v0.13.0, on a real LibreOffice-produced tagged PDF with
**14 `/StructElem`**: a **no-op** write — read, validate, optimize, write, changing
nothing — returns a document with **zero** of them while `/StructTreeRoot` and
`/MarkInfo /Marked true` survive. The read half is sound; `WriteContext` does not serialise
the objects the tree points at.

So **eight shipped operations were emitting that exact lie**: `Rotate`, `Optimize`,
`SetLang`, `StripMetadata`, `StripActive`, `RemoveFilesAndMedia`, `InsertBlank`,
`NormalizePageSizes`. None was on anyone's list. They were found by law 2's guard on its
first run, which is the argument for law 2 in one sentence.

## The law is a POST-CONDITION, deliberately

`writeMutated` and `honest()` each ask, after the write: *did this destroy the structure
while leaving the claim?* — and drop the claim when it did. They do not strip
unconditionally.

That shape matters because it **expires on its own**. The day the write path carries a tree
— which is `/pending 467`, and P05's prerequisite — the check stops firing, with no line
anyone has to remember to delete. An unconditional strip would have to be found and removed
by whoever builds tagging, and would silently destroy their work until they did.

## Why the guard enumerates rather than lists

ADR-009's rule: the guard checks the door, not the sites that happen to be right. The
population comes from `go/ast` over the package — every exported function whose first
parameter is a document and whose first result is bytes.

**Its first filter was itself a hole, and that is recorded because it is the instructive
part.** It required the first parameter to be *named* `pdf`; five operations were hiding
behind a different name, including `RedactPages` — the very operation the plan singles out
as needing an explicit verdict. An operation escaping a census by what it calls its
argument is the census failing, not the operation qualifying. The filter is now the shape,
and the population went 28 → 33.

## What this ADR does NOT say

It does not say nib produces accessible documents. It says nib does not lie about it.
Authoring structure is `PLAN-accessibility.md` P05 onward and is blocked on the write path
carrying a tree at all. **Read-aloud (`/pending 408`, v1.129.7) is not this either** — a
screen reader needs a tag tree and read-aloud builds none.

## Alternatives considered

**Tag the composed page instead of dropping the claim** — for `nup`, this means authoring
structure for a page that did not exist a moment ago. That is P05's machinery, four phases
later, and D2 is explicit that preservation precedes authoring: *"tagging authored on top
of a pipeline that eats tagging produces documents that are accessible until the user
rotates a page."*

**Score the output with veraPDF instead of asserting structurally** — refuted by
measurement. Dropping a claim *adds* PDF/UA failures, because UA-1 requires a structure
tree: an untagged file fails 6.2 t1 and 7.1 t11 by construction. A conformance failure
count scores honesty as a regression, so it cannot express law 1.
