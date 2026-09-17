# ADR-038 — `/Stm` is a marked-content reference's key, and content that moved into a form is reached through one

**Status:** accepted
**Date:** 2026-09-16
**Context:** `PLAN-ua-coverage.md` P02.S08, D5. Extends ADR-034 and ADR-031.
**Applies:** `pdfops.NUp` and `Booklet` through `carryTagsThroughNUp`, the structure-tree model
(`readStructTree`), the text reader (`readPageRuns`), the reviewable view (`readStructureView`), and every
future operation that moves page content into a Form XObject.

## Decision

**A structure element's marked-content kid says which content stream its MCID lives in, and the only
dictionary allowed to say it is a marked-content reference.**

Three rules, and the third is the one that is easy to get wrong twice.

1. **`/Stm` goes on an MCR and nowhere else.** ISO 32000-1 Table 323 enumerates a structure element
   dictionary's keys — `Type, S, P, ID, Pg, K, A, C, R, T, Lang, Alt, E, ActualText` — and `/Stm` is not among
   them. Table 325 does the same for an object reference: `Type, Pg, Obj`. Only Table 324, the marked-content
   reference, defines it: *"(Optional; shall be an indirect reference) The content stream containing the
   marked-content sequence. This entry should be present only if the marked-content sequence resides in a
   content stream other than the content stream for the page."* A key written outside its own table is not
   redundant, it is **inert**: a conforming reader ignores it and falls back to exactly the behaviour the key
   was written to prevent.

2. **A bare integer kid is a claim about WHERE, not only which.** §14.7.4.2 admits the integer form only
   *"in the common case where the marked-content sequence is contained in the content stream of the page that
   is specified in the Pg entry of the structure element dictionary."* An n-up makes that false for every
   element it carries: the sequence is inside a Form XObject and the sheet's own stream holds nothing but the
   `Do` that draws it. So an operation that moves content into a form rewrites the integers it carries as
   MCRs, or it has not carried them.

3. **An MCID is unique within a content stream and nowhere wider, so a reader keys on the stream.** Two forms
   on one sheet both carry an MCID 0 — that is the ordinary shape of an n-up, not a pathology — and a reader
   keyed on `(page, mcid)` hands both texts to both elements. `structKid` carries `stm`, `textRun` and
   `markedSeq` carry the object number of the stream they were read from, and `readStructureView` keeps a
   second index keyed by stream that **only a kid naming one consults**. A kid that names a stream is read in
   that stream and no other, with no fallback: a miss is empty text, and falling back would restore the merge
   silently.

**Zero means the page's own stream, not "unknown"**, in `structKid.stm` and in `textRun.stm` alike — the spec
says so from both directions (Table 324's *"If this entry is absent, the marked-content sequence shall be
contained in the content stream of the page identified by Pg"*, and §14.7.4.2 above). A stream held directly
rather than as an indirect object is also 0, because `/Stm` *"shall be an indirect reference"* and so can
never name one: unnameable and page's-own are the same answer to the only question asked of the field.

## Why this needed deciding at all

**Because the previous shape passed every check this repo owns.** `carryTagsThroughNUp` wrote
`d["Stm"] = pl.xobj` onto the *element* from P01.S06 until now, and nothing could see it:

- **veraPDF scores both shapes identically.** Measured on a 61-element fixture and its `NUp(2)`: `5 t1`,
  `7.2 t33`, `7.2 t34` for the source, the broken carry and the repaired carry alike — every one of them the
  fixture's missing `/Lang`, none of them the carry.
- **`nib ua` is blind by construction.** `uacheck`'s tree walk discards every marked-content kid before it
  looks at the shape (`structure.go:201-202`), and `7.2 t34` resolves content→element through
  `/StructParents` — the reverse direction, which has always worked.
- **The repo's own completeness gate is indifferent.** `structureCarriedCompletely`'s six conditions ask about
  `/Pg` liveness, `/ParentTree` ownership and form draw counts; none reads a kid's encoding.

What it cost was invisible to all three and plain in nib's own tool: **31 of 61 elements read the wrong
text** after the carry, two source pages' worth concatenated. Element 27 read
`"TitleParagraph number 30 with some words…"` where its source read `"Title"`, with a `rect` unioned across
two pages.

## What was rejected

**A test-only oracle** that tokenizes the form stream directly and matches MCIDs, leaving the production
reader alone. It would have proved the *document* correct while `nib tag tree` and the Tags panel went on
reading nib's own n-up output wrongly — and `drawForm` already computed the stream's object number and threw
it away (`textrun.go:768` at `e637cb3`, the commit before this one; `:822` here, where it is now kept), so
the oracle would have been a second implementation of a door that exists (ADR-009).

**A signature-stable MCID renumbering** — inlining each form's content into the sheet's stream and renumbering
the MCIDs so bare integers stay honest. It authors content rather than references, which is the line D2 draws
and the whole reason the carry is allowed to exist.

## Consequences

- `structartifact`'s `errCommitInForm` refusal (`structartifact.go:79`) **now fires on documents nib itself
  produces.** It was never dead in the suite — `structartifact_test.go:310-313` has driven a hand-built
  `/K << /Type /MCR /Pg 3 0 R /MCID 0 /Stm 5 0 R >>` through it all along — but it dereferences the kid, and a
  bare integer is not a dictionary, so an n-up's form-resident element used to miss it and be refused later,
  and more expensively, by a full page scan. The sentinel is the same from both sites and the message is not:
  `(element N, /MCID M)` against `(page N, /MCID M)`.
- The cost is **not** what the arithmetic said, in magnitude or in sign. 61 direct MCR dicts of ~45 bytes each
  projected ~2.7 KB, about +2.8%. Measured over two populations driven through the real binary — 61 elements
  over 2 pages, and 241 over 8 — the range is **−0.33% to +0.05%**: 94,405 → 94,435 at `n=2`, and
  103,620 → **103,281** at `n=4`, where the output gets *smaller* because 241 elements each shed a `/Stm` key
  and near-identical dictionaries in an object stream compress to almost nothing. The range is what is
  recorded, because the failure mode is projecting past it; both populations are nib's own Markdown
  conversion, and a document from another producer is outside what was measured.
- MCRs are written **direct**, not as indirect objects: the spec's own Example 2 writes one direct, it costs no
  xref slot per kid, and pdfcpu's `/K` array validation skips an indirect entry it has already marked valid
  (`validate/structTree.go:139-152`) while a direct dict is always walked — so the shape that gets validated is
  the shape that was written.
- **ADR-031's parenthetical is now stale and is corrected here rather than there.** It describes the carry as
  writing *"three keys (`/StructParents`, `/Pg`, `/Stm`)"* (`031-nothing-claims-tagging-it-has-not.md:172`).
  It now writes `/StructParents` and `/Pg`, and rewrites kids. ADR-031's **decision** — nothing claims tagging
  it has not — is untouched and still holds, so it is not superseded and not edited; an ADR is immutable in
  its decision content and this is the place a later record corrects an earlier one's incidental detail.
- **No Linux screen reader can observe this.** Measured 2026-09-16: the text Evince exposes over AT-SPI, which
  is what Orca speaks, is byte-identical for the broken and the repaired carry, because poppler extracts
  geometrically and never opens the structure tree. The consumer that does honour the rule is pdf.js — nib's
  own viewer, and Firefox's — which reads `/Stm` on an MCR and has no branch that would read it off an
  element. **That absence is a search, not an impression**: `grep -o 'getRaw("Stm")\|"Stm"'` over
  `web/vendor/pdfjs/pdf.worker.min.mjs` returns exactly **one** hit, inside the `"MCR"===n` branch. That is the declared limit on the claim, and the reason the slice is graded on the document rather
  than on a listening test.

## What this cost to get right

Written and reviewed the same day, and **the review found three criticals in it**, none of which any tier
could see: a `/K` spelled as a single dictionary — ISO 32000-1's own Example 2, and what LibreOffice and Word
emit for a single-kid element — reached `DereferenceArray`, came back a wrong-type error, and dropped the
**entire** tag tree; a run was stamped with the stream its glyphs were drawn in rather than the one its
sequence was opened in, which files text under a stream no `/Stm` can name; and an element that *inherited*
`/Pg` was skipped, keeping bare integer kids in the half-converted tree the whole design refuses. A fourth
finding was a test that executed **zero** assertions, proven by running it rather than by reading it.

Then a red-proof pass produced three survivors. Two were the probe's fault — an inert branch and a fixture
whose ordering could not reach the defect — and one was found only by a **blind** adversary shown the
production code and none of the tests: the per-stream rectangle read without the guard its per-stream text
keeps, which every assertion in the slice survived because text is asserted far more often than geometry.
That is the mutation a targeted pass structurally cannot find, since the model that picks a mutation from the
predicate is the model that would have written the test.

The lesson is the one the decision itself records: the old shape passed everything, and so did four of the
first versions of the things written to replace it.
