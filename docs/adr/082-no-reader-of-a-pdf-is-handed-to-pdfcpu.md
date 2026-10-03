# ADR-082 — no reader of a PDF is handed to pdfcpu; its reader-taking `api` functions are restated in `pdfread`

**Status:** accepted. `/pending 716`, `/pending 717` (2026-10-03). Supersedes ADR-055 decision 1 in part — the
`Reader` door, and `OptimizeOrRefuse` for `mdpdf`'s exhibits — and decision 4's description of the guard.

## Context

ADR-055 gave pdfcpu's reader-taking `api` functions a door, `pdfread.Reader`: an unvalidated second parse that ran the
reference check and then handed the function its reader. The function then did its own read — and all but three
(`NUp`, `PageDims`, `MergeRaw`'s reads) its own **optimize pass, with no budget**. Measured on `/pending 706`'s 400-form
chain (68 KB), every operation over one of them was still running at 30 s: `Outline`, `SplitByBookmarks`,
`StampTextLayer`, `StampImages`, `FillFormCSV`, `FillFormXFDF`, `ExportFormJSON`, image extraction, `Encrypt`,
`RemovePassword`, `RedactPages` (through `MergeRaw`'s closing pass over the merged context) and `mdpdf`'s packet (whose
cover is only validated). The door also cost each of them a second parse.

The guard, `TestEveryValidatingReadRoutesThroughTheDoor`, looked for a `bytes.NewReader` built in the same function as
an `api` call, and so could not see a reader built in one function and handed to pdfcpu from another (`/pending 717`).

## Decision

1. **Every reader-taking `api` function nib uses is restated in `internal/pdfread/apiread.go`** over `Validated` or
   `ReadOptimized`, doing with the context exactly what the `api` function did (each cites its pkg/api line, v0.13.0).
   One parse, and the budgeted pass. `Reader` is deleted.
2. **Image extraction refuses past the budget** (`ReadOptimizedOrRefuse`): pdfcpu finds a page's images in what the
   pass records, so a skipped pass would answer "no images".
3. **`MergeRaw`'s closing pass goes through `Optimize`**, budgeted over the merged context — which no part's own
   budget bounds. So `mdpdf`'s exhibit normalization skips an unaffordable pass instead of refusing the exhibit:
   the refusal existed only because the merge it admitted the exhibit to was unbudgeted. `OptimizeOrRefuse` had no
   other caller and is removed.
4. **The guard bans the functions, not the readers.** It reads pdfcpu's `api` source the module builds against and
   collects every exported function with an `io.ReadSeeker`/`[]io.ReadSeeker` parameter or an `inFile…` path; outside
   `pdfread` such a function may be named only in a call passing literal `nil` at every reader position
   (`api.Create`, `api.ImportImages`), or be `api.ReadContext`, the unvalidated read. Where a reader was built no
   longer matters, because no reader of a PDF may reach pdfcpu at all.

## Consequences

- Each of the operations above finishes on the 400-form chain in 27-290 ms on a loaded machine
  (`TestEveryOperationOverAnApiReadFinishesOnHostileForms`, `TestAPacketOfDocumentsWhoseOptimizePassIsUnaffordableFinishes`).
- A pdfcpu upgrade that adds a reader-taking function is caught by name the first time nib calls it.
- **Declared:** a restatement can drift from the `api` function it restates on a pdfcpu upgrade; each cites the line it
  copies, as `Validated` always has.
