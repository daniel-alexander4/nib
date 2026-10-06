# docs/red-proofs.md, tier 1: "The documents are closed when the last window goes, so a reload comes back empty (ADR-085, v1.183.0)"
#
# The defect: the close is moved to the 1->0 window transition — the obvious place for "closing Nib
# closes its documents", and the alternative ADR-085 refuses, because a reload is that transition too.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -count=1 -run TestAReloadKeepsItsDocuments"
EXPECT="a refresh closed"
