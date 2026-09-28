# ADR-055 — pdfcpu validates and optimizes only through `internal/pdfread`

**Status:** accepted. `/pending 675`, `/pending 714`, following `/pending 706`.

## Context

pdfcpu's validator and its optimize pass are unbounded on hostile input, and neither can be bounded by a
pdfcpu option (v0.13.0: `ResourceLimits` reaches neither).

- **The validator recurses on `/UseCMap`** (`validateCMapStreamDict` → `validateUseCMapEntry`). Two embedded
  CMaps naming each other — or one naming itself — recurse until Go's stack limit, and a stack overflow is
  not a panic: `recover` cannot catch it, and the PROCESS dies with the user's unsaved documents. Measured:
  a 2.5 KB file, about 5 s, and **opening it killed nib** — the server's Open computes the document's flags
  through a validating read.
- **The optimize pass is cubic in a chain of forms and exponential in shared resources** (`/pending 706`: 68 KB,
  90 s; forty self-naming forms, more than four minutes). /pending 706 bounded it for `internal/pdfops`
  only; the accessibility checker (`internal/uacheck`) could not import that door and still ran the pass
  unbounded (`/pending 714`).

## Decision

1. **`internal/pdfread` is the one door** through which nib hands bytes to pdfcpu's validator or its optimize
   pass. `Validated` is pdfcpu's read-then-validate with the `/UseCMap` loop refused in between, on the same
   parsed document; `Reader` gives a pdfcpu `api` function its reader only after an unvalidated read shows no
   loop; `ReadOptimized` / `Optimize` run the pass only when nib's estimate says it is bounded
   (`Unaffordable`), and a page operation skips an unaffordable pass rather than failing.
   `OptimizeOrRefuse` is for a caller whose pass is the point: `mdpdf`'s exhibit normalization, which proves a
   document survives a merge that then runs pdfcpu's pass unbudgeted, so skipping there would admit the hang.
2. **The checker refuses rather than skips** (`ReadOptimizedOrRefuse`, `ErrUnaffordable`): its rules were
   calibrated against the optimized reading — measured, four verdicts move without the pass — so a report
   from an unoptimized reading is one nothing vouched for.
3. **The loop check examines every stream**, not only those a font reaches, so it needs no second copy of
   pdfcpu's reachability; a document with an unused looping CMap is refused, and such a file is malformed.
   Opening such a document still shows it; every operation refuses with the loop named.
4. `TestEveryValidatingReadRoutesThroughTheDoor` is an AST census over the module: outside `pdfread` nothing
   NAMES pdfcpu's validating reads, its optimize pass or any `api.*File` function — a call, a package-level
   variable or a function value alike — imports its `validate` package, or hands pdfcpu a `bytes` or `strings`
   reader it built itself (a conversion into `io` excuses nothing). Its exemptions (JSON and image readers)
   are named. Red-proved with six bypass shapes the first version of the census passed.

## Consequences

- The less frequent operations that call pdfcpu's `api` wrappers (N-up, stamps, bookmarks, forms,
  encryption, image extraction, merge) pay a second parse for the loop check — measured +30-85% on 100 KB to
  6 MB files. The two hot reads (`FlagsJSON`, on every document change, and `PageCount`) were restated over
  `Validated`/`ReadOptimized` and parse once; `FlagsJSON` still costs more than before — +3% to +22% on 0.9 to
  6 MB producer files, measured — which is the budget estimate (0.4-1.8 ms) and the loop check (0.1-2.5 ms).
- **A document with a looping CMap nothing draws is refused whole**, and since the server's Open swallows the flags
  read's error, such a document opens with its embedded Nib flags silently ignored. Declared, not closed.
- **Those wrappers still run pdfcpu's optimize pass inside themselves, unbounded** — the 706 class, reached
  only by the loop check. Declared; filed with this change.
- pdfcpu's validator may hold other unbounded recursions; only `/UseCMap` was measured.
- The census cannot see a reader built in one function and handed to pdfcpu from another; none exists today.
- **ADR-052's CMap-reader guard gains one named exemption, `testpdf.cmap`**: it WRITES the fixture CMaps the loop
  tests are built from and reads none, so it is not the reader ADR-052 routes through `internal/fontcode`.
