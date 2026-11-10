# docs/red-proofs.md, tier 1: "A removed picture stays in the file through a parent /Pages node's resources (PLAN-image-edit P01.S02, D3's prune)"
#
# The defect: one condition of the picture edit's door is taken out; the named test must go red for its own reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAPictureNamedOnlyByAParentPagesNodeLeavesTheFile"
EXPECT="held by the parent /Pages node's resources"
