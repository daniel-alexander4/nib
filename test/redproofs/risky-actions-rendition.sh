# docs/red-proofs.md, tier 1: "`Rendition` removed from `riskyActions`"
#
# The defect: a rendition action can carry its own JavaScript (ISO 32000-1 §12.6.4.13), so
# dropping it from the map makes Scan report a document that runs code as clean. The check
# names the missing type AND the count mismatch — two failures, because the list and the map
# are maintained separately on purpose.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestRiskyActionsCoverTheTypesThatCanRunOrHide"
