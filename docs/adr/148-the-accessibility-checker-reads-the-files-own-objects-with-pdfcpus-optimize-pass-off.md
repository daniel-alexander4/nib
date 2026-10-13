# ADR-148 — the accessibility checker reads the file's own objects, with pdfcpu's optimize pass off

**Status:** accepted
**Date:** 2026-10-12
**Supersedes:** ADR-074 decision 3 only ("form and font fusion stay on"). ADR-074's decisions 1, 2 and 4 stand: the
per-page resource step stays off, inherited `/Resources` are still put on each page by `pdfread.InheritResources`,
and content names still go through `nameKey`.
**Extends:** ADR-052 (the checker reads as veraPDF does). ADR-009 (one configuration, `checkerConfig`).
**Context:** `/pending 672`. pdfcpu's optimize pass fuses equal form XObjects and byte-identical font dictionaries
into one object when it opens a file. veraPDF reads the file's own objects. So wherever a fusion could move a
count, the checker's count was not veraPDF's, and it carried machinery to know when: `formTwins` re-asked pdfcpu's
own equality predicate over every equal-length form in the file (16,384 comparisons at most, then a refusal), `7.20
t2` refused any keyed form with a twin and any form with a twin that draws another, the TrueType group rule refused
whenever the pass had merged any font dictionary, and the read itself was refused when the pass — which has no
budget of its own — would not finish. All of that was correct and uninformative, and it existed only because of
the fusion. With `optimize` off, two equal outer forms each drawing a keyed inner read as two, and nib reports
Fail — veraPDF's answer — where the fused reading saw one outer traversed once: a false pass that only the
refusal was covering.

The reason it was not done at the P06 phase close was that every rule in the package had been measured with the
pass on. That measurement was made again, on this tree, both ways:

| instrument | optimize on | optimize off |
|---|---|---|
| the oracle, law 5 (`TestTheOracleValidatesTheChecker`), 701 documents | 73,605 of 73,605 pairs strict | 73,605 of 73,605 |
| real producers (`TestTheCheckerAgreesWithVeraPDFOnRealProducers`), 36 files | 3,780 of 3,780 settled and agreeing, 0 refused | 3,780 of 3,780, 0 refused |
| per-clause reach on those files, 105 clauses | — | identical, clause for clause |
| veraPDF's own corpus, 297 files | 30,931 pairs, 0 false pass, 0 false fail | 30,931 pairs, 0 false pass, 0 false fail |
| `knownCannotCheck` rows | — | 5 answered (the twin MacExpert fonts: 7.21.4.1 t1/t2, 7.21.5 t1, 7.21.6 t1/t4) |

No clause is worse. (Both runs were taken while the machine ran another suite; they are counts, not timings.)
**Applies:** `internal/uacheck`.

## Decision

**1. `checkerConfig` sets `Optimize = false`.** The object numbers every rule sees are the file's.

**2. What existed only because of the fusion is removed, not left inert:** `formTwins`, `twinEqual`,
`maxTwinCompares` and the `Document` fields behind them; `7.20 t2`'s two twin refusals (a keyed form reached twice
is a Fail and nothing else is asked); the TrueType group rule's `DuplicateFonts` refusal.

**3. The checker no longer refuses a document the optimize pass could not afford.** It runs no pass.
`pdfread.ErrUnaffordable` stays for the reader that still depends on one (`pdfread`'s image extraction).

**4. The other read-shaping fields stay pinned** (`OptimizeResourceDicts`, `OptimizeDuplicateContentStreams`,
`ValidationMode`, …), so the reading still does not depend on the user's pdfcpu `config.yml` — whose own default
for `optimize` is now the opposite of the pin, and `TestThePdfcpuConfigOnThisMachineDoesNotMoveTheAnswer` holds it.

## Consequences

- Five tests that pinned a refusal pin veraPDF's answer instead: two equal keyed forms drawn once each Pass, one
  of them drawn twice Fails, two equal outer forms each drawing a keyed inner Fail, two keyed forms equal only one
  way round Pass (that row's answer had depended on map order, 27 runs of 40), and two identical fonts sharing a
  program take veraPDF's recorded verdicts on every TrueType clause. Each goes red with the pass switched back on.
- A report is no longer withheld because of a duplicate object the document happens to carry.
- The read is cheaper by the whole pass. Not measured here as a time: the machine was loaded.
- **Declared:** `internal/pdfops` and every other reader still read with the pass on (`pdfread.ReadOptimized`,
  `ReadForInspection`), so an object number the checker reports is the file's, and may differ from the number
  another nib surface shows for a fused twin. Nothing joins the two.
- **Declared:** the door is still named `pdfread.ReadOptimizedOrRefuseSettingAside`. It optimizes exactly when the
  configuration says so, and the checker's says not.
