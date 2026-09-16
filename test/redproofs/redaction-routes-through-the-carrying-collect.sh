# docs/red-proofs.md, tier 1: "Redaction routed through the carrying Collect" (P02.S03's debt, v1.129.142)
#
# The defect: `RedactPages` builds its runs of untouched pages through the CARRYING page-subset door,
# so the redacted document carries the source's structure tree — and structure elements describe what
# the page SAID. A reader walking that tree recovers the headings, the reading order and the shape of
# the very content the raster replaced, and `/Alt` or `/ActualText` on a grouping element can carry
# the words themselves.
#
# **P02.S03 recorded this as a debt: probe the backstop red once `Collect` carries.** Discharging it
# corrected the test. The first probe stayed GREEN: rasterising page 1 puts an `ImagesToPDF` output
# first in the segment list, and `api.MergeRaw` keeps the FIRST document's catalog, so the merged
# result had no tree however the untouched run was collected — the door under test ran and the
# assertion could not see it. Rasterising the SECOND page makes segment 1 the untouched run.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestRedactionEmitsNoStructureTree -count=1"
EXPECT="carries a structure tree"
