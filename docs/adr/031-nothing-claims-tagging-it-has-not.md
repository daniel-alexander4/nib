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

## Nothing violates it today, and the story of finding that out is the useful part

**CORRECTED 2026-09-11, the same day this ADR was written.** The first version of this
section said a no-op pdfcpu write destroys every struct element, and that eight shipped
operations were emitting a false claim. **Both were artefacts of one mistake**, and it is
recorded here rather than quietly fixed because the mistake is more instructive than the
law.

The evidence was `bytes.Count(pdf, []byte("/StructElem"))`. **pdfcpu writes the structure
tree into a compressed object stream**, so that count is `0` for every pdfcpu output
whatever it contains — a perfectly tagged document and a stripped one are identical to it.

Parsed instead, on a LibreOffice document with 14 struct elements:

| | claims tagging | struct elements |
|---|---|---|
| source | yes | **14** |
| no-op write | yes | **14** |
| `Rotate(90)` | yes | **14** |
| `Optimize` | yes | **14** |
| `NUp(2)` | no | 0 |
| `Collect(1)` | no | 0 |

**Nothing lies.** `Rotate` and `Optimize` carry the tree; `NUp` and `Collect` drop the claim
and the content together, which is exactly what law 1 asks for. The enforcement written
against the byte count was **stripping trees that had survived intact**, and it shipped for
the length of one session before this measurement caught it.

**So law 1 is a standing rule with no current violation, and that is a legitimate thing for
an ADR to be.** What is kept is the census (law 2) and a guard that would catch a violation
if one appeared; what is gone is any claim that one exists, and the code that acted on it.

## What the byte count could not see, stated so nobody repeats it

A raw `/StructElem` count answers a question about the FILE FORMAT — *does this literal
appear uncompressed* — and was being read as a question about the DOCUMENT. Every
downstream conclusion inherited that: a plan decision superseded, a pending item filed, four
slices built, and an enforcement that destroyed user data.

The rule it breaks is one this repo already had: *a claim about code is a hypothesis until
you have seen the line.* A byte count over compressed output is not seeing the line.

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

It also does not say that nib's operations *preserve* tagging as a rule. Two measured ones
do and two do not; the census records which, per operation, and that is the whole of the
claim.

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
