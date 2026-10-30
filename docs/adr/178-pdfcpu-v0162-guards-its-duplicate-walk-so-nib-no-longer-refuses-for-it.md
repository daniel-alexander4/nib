# ADR-178 — pdfcpu v0.16.2 guards its duplicate walk, so nib no longer refuses on its behalf

**Status:** accepted
**Date:** 2026-10-30
**Extends:** ADR-055, ADR-069, ADR-077 (the guards written against v0.13.0's optimize pass); ADR-174 (pdfcpu as published).
**Context:** `/pending 920` F11. `Unaffordable` refused (non-strict reads skipped the pass, strict ones and `ExtractImages`
refused) a document with a loop under a font or image pdfcpu could merge (`duplicateLoop`, `/pending 867`) and one with an
XObject whose `/Subtype` is a reference (`unnamed`). v0.13.0 recursed through a duplicate's references remembering nothing
and dereferenced a nil name. `pagebalance.go` made a quadratic per-page step linear. Measured on v0.16.2 (2026-10-30, each
shape in its own process): the duplicate walk marks an object before it descends and stops at `MaxRecursionDepth` 100
(`optimize.go:1161-1177`), and the pass returns nil over a self-naming dict, rings of 3 and 90, equal images over a loop,
equal Type 3 fonts naming themselves, an 80-level doubly-shared graph, both SHA-1 collision PDFs and the old skip fixture.
Flat page trees of 2,500 / 5,000 / 10,000 pages read in 255 / 463 / 751 ms without the balancer (339 / 502 / 813 with it).

**Decision.**

1. **Deleted:** `duplicateLoop` with `loopingFontsAndImages`, `dupPrint`/`dupPrinter` and `levelWalk.loops`; the `unnamed`
   refusal; `pagebalance.go` and its call. A document these skipped or refused is now optimized.
2. **Kept, with the measurement that keeps them:** `pages.go` / `pagesim.go` — v0.16.2's `PageDict` is still quadratic on a
   flat tree (2.3 s / 10.4 s / 40.2 s for every page at 2,500 / 5,000 / 10,000, against 16 / 27 / 80 ms for `Pages`);
   the `/State` + `/StateModel` set-aside (ADR-168) — v0.16.2's relaxed validator accepts a non-standard model with a
   non-empty State but still refuses an empty State and an invalid State under `Marked`/`Review`
   (`validate/annotation.go:206`, `:218`, `:230`); `refuseDeepReferences` — `EqualObjects` has no depth check.
3. **`testpdf.PassSkipped`** is a form over a 9,000-link chain (past `maxShapeDepth`), the one skip trigger left that needs
   no large document.

**Declared.** A ring or chain of more than 100 references under two equal fonts used to read with the pass skipped; it now
answers pdfcpu's own "max recursion depth exceeded", as a chain with no loop already did
(`TestAChainDeeperThanPdfcpusBoundUnderADuplicateIsPdfcpusError`). Evidence is hand-built shapes on flat trees; the
population run (`build/pdfcpudiff.sh`) over the formerly skipped documents is not made.
