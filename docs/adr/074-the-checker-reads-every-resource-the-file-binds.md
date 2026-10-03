# ADR-074 — the accessibility checker reads every resource the file binds, never pdfcpu's pruned set

**Status:** accepted. `/pending 782` (2026-10-03). Amends the reading ADR-052 and `/pending 714` describe: the checker's
pdfcpu configuration keeps the optimize pass (form and font fusion) and turns OFF its per-page resource step.

## Context

The checker read each document through pdfcpu with `OptimizeResourceDicts` on. That step (`consolidateResources`)
prunes every page's `/Resources` to the names pdfcpu's own content scan finds — and that scan does not decode a PDF
name's `#xx` escapes. A page drawing `/X#30 Do` (which spells `/X0`) kept a resource named `X#30`, which does not exist,
and DELETED `/X0`, the one it draws. No reader downstream could then resolve the name, however correctly it decoded it:
nib answered CannotCheck where veraPDF resolves the form and fails 7.20 t1. The same pruning hid named `/Properties` and
patterns spelled with escapes.

## Decision

1. **`checkerConfig` sets `OptimizeResourceDicts = false`.** Every reader in `internal/uacheck` looks a resource up by
   the name the content stream uses, so the unpruned dictionary answers every lookup the pruned one did, and the ones it
   could not.
2. **The step's other half — putting inherited `/Resources` on each page — is kept**, as nib's own door
   `pdfread.InheritResources`, which `open` and `pdfread.ReadForInspection` both call. It never prunes.
3. **Form and font fusion stay on.** They are `optimizeFontAndImages`, which no setting gates, so `formTwins` still sees
   the fused forms it exists to account for.
4. **Content names are decoded through one door, `nameKey`**, at every site in the content walk (marked-content tags,
   `/Properties`, inline `/MCID`/`/Lang`, `Do`, `scn`, `Tf`, `gs`), held by `TestTheContentWalkReadsANameThroughOneDoor`.

## Consequences

- Measured against veraPDF 1.30.2 on ten escaped-name fixtures: every row agrees. Both local corpora stay green.
- Four inherited-binding rows that read CannotCheck ("pdfcpu drops the binding") now return veraPDF's recorded answers.
- A page whose `/Contents` decodes past the budget is now CannotCheck from the content walk itself, where the pruning
  pass used to refuse the whole read as unaffordable first.
