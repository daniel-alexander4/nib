# ADR-116 — the checker carries two fonts across pdfcpu's validator

**Status:** accepted
**Date:** 2026-10-09
**Context:** `/pending 857`, split from `/pending 612` and widened by `/pending 683`'s measurement. ADR-115 gave the
read door a step that sets entries aside for `api.ValidateContext` and puts them back, and used it for four text
keys. Two FONT shapes had the same problem in a worse form — the validator does not refuse them, it removes them:
- a Type 3 font dictionary written DIRECTLY in a `/Font` dictionary is dropped. The checker noticed
  (`hasInlineType3Font`) and refused every clause that reads page content — 23 of them — rather than answer over a
  font it could not see;
- a `/FontFile3` whose stream's `/Subtype` is not a name (a wrong type, `null`, or absent): the validator drops the whole font from the page's
  resources (a CIDFontType0), or refuses the document (`validateFontFile3SubType`, a Type 1 font). veraPDF gives no
  report for the first — it throws — and reports on the second.
**Supersedes:** ADR-115's consequence *"A Type 3 font written directly in `/Resources` is still DROPPED … this step
does not see it"*, and its function name: `setAsideMistypedText` is `setAsideForValidator`.
**Applies:** the PDF/UA checker's read.

## Decision

**1. Both entries are set aside for the validation and put back** (`fontsTheValidatorLoses`, called by
`setAsideForValidator`): the inline Type 3 font's entry in its `/Font` dictionary, and the `/FontFile3` entry of the
descriptor. Found from every object in the cross-reference table, following direct objects only.

**2. Nothing new decides what they mean.** The rules read a restored Type 3 font as they read one written as its
own object, and a restored `/FontFile3` through `fontFile3SubtypeThrows` (a CIDFont: veraPDF reports nothing) or as
no program (a simple font).

**3. `hasInlineType3Font`'s refusal stays** as the backstop for a font this search does not reach.

**4. Before the validation, an object is looked up with `Dereference` and never `DereferenceStreamDict`.** The second
marks the object VALID as it returns it (pdfcpu `xreftable.go`, v0.13.0) and the validator skips what is marked. The
first cut of this step looked the `/FontFile3` stream up that way, and the documents became readable with nothing set
aside — an exemption from validation nobody had chosen, which also hid that a `null` or absent `/Subtype` needed
carrying. It was found only because switching the set-aside off turned no test red.

## Consequences

- An inline Type 3 font: 23 refusals become answers. Each of three documents answers every clause exactly as its
  indirect twin does — an invariant that needs no oracle — and as veraPDF does
  (`TestAType3FontWrittenInlineAnswersAsItsIndirectTwinDoes`).
- A CIDFontType0 with a mistyped subtype is refused by all 105 clauses ("veraPDF reports nothing"), where only its
  font clauses refused; a Type 1 font so written opens and fails 7.21.4.1 t1 as veraPDF does
  (`TestAFontFile3SubtypeThatIsNotANameReportsNothing`, whose own tripwires — "if the font now survives the read,
  move the row" — were what reported the change).
- Still not for a context that is written out (ADR-115 §2).
