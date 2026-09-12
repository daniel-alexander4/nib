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
`carried`, `dropped`, `partial` or `untouched` — in one table, and a guard asserts both that
every operation *has* one and that the one it has is **true**, enumerating the population
from the code.

**(AMENDED 2026-09-11, v1.129.16 — a widening of this law, not a reversal of it, so it is
made in place rather than superseded.)** Two things were missing. `partial` — a live tree
that does not reach every page with content — is a state the original three words could not
express, so the operation that produces it (`Append`, with the tagged document first) had to
be declared as something it is not. And the guard asserted only that a verdict *existed*: it
never compared a declaration to a document, so **19 of the 33 rows were false** — every one
declaring `dropped` about an operation that carries the tree intact — and nothing was red.
A census that cannot be wrong is not a census.

## Why a false claim is worse than a visible loss

A screen reader told a document is tagged **stops reaching for the fallbacks it would
otherwise use**. A user who can see their tagging is gone can re-tag, re-export, or choose
a different tool; a user handed a document that asserts structure it does not have has had
their own check defeated. That asymmetry is the whole of law 1, and it is why the remedy
is dropping the claim rather than preserving it — honesty is reachable today and
preservation is not.

## One operation violates it, and the story of finding that out is the useful part

**CORRECTED TWICE, both on 2026-09-11.** Read the second correction first: the section below
is the first correction, and it was itself wrong.

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

~~**So law 1 is a standing rule with no current violation.**~~

**(STRUCK, second correction, v1.129.16.** `NUp` violates law 1 and has always violated it,
including in the released v1.129.4. The row above says `NUp(2) | no | 0`; measured again and
repeatably, on both the committed fixture and a 4-page LibreOffice document, the raw n-up
output keeps `/Marked true` and a **45-element** `/StructTreeRoot`.**)**

**What made it invisible is the more useful half.** The corrected predicate was *"claims
tagging and has zero struct elements"* — the weakest available reading of law 1 — and
`api.NUp` composes its sheets as NEW page objects while carrying the source catalog onto
them. Every one of the 45 elements points at a page that is no longer in the page tree, no
composed sheet carries `/StructParents`, and the element COUNT is therefore 45, not 0. **The
predicate written to enforce law 1 could not see the only operation in the package that
breaks it.** So the count was corrected and the question was still the wrong one.

**veraPDF names the law in its own words, and it is outside this repo entirely.** ua1 against
the same documents: the source and `Rotate`'s output fail the same three clauses (5 t1,
7.1 t9, 7.1 t10 — producer limitations); the n-up output adds **7.1 t3, *"Content shall be
marked as Artifact or tagged as real content"*, with 24 failed checks.** Reaching for the
third oracle is what should have happened the first time.

**The remedy is a post-condition, and that is the entire difference between the version that
destroyed data and this one.** `honest` parses the output and drops the claim only when the
tree anchors to nothing — nothing via an element's `/Pg` pointing at a live page, *and*
nothing via any page's `/StructParents`. It refuses to act while either linkage survives. The
reverted version stripped unconditionally, so it ran over documents whose trees were fine.

**And the remedy was measured too, because a remedy is a claim.** Stripping the n-up output's
claim does not remove 7.1 t3 — the content is untagged either way — and adds 6.2 t1 and
7.1 t11, since PDF/UA requires a structure tree. Honesty scores as a regression on a
conformance count, which is why the acceptance criterion is structural.

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

It also does not say that nib's operations *preserve* tagging as a rule — though far more of
them do than this ADR first claimed. **Measured: 19 carry the tree, 10 drop claim and content
together, 1 is `partial`, and 1 (`NUp`) had to be fixed.** The census records which, per
operation, and that is the whole of the claim.

**`partial` is recorded and not enforced, and that is a decision with a named cost.**
`Append` with the tagged document first keeps a live 45-element tree and leaves the appended
page undescribed. Dropping the claim there would destroy the whole tree to fix one page, and
`p2p/readme.go` and `p2p/sigpages.go` take that path for **every ceremony document** — so the
strict reading of law 1 costs more accessibility than it buys. Whether law 1 should be
superseded to permit a structure tree over a partially-described document is open and is
recorded at `PLAN-accessibility.md` D9.

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
