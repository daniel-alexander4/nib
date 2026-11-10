# docs/red-proofs.md, tier 1: "A Figure that holds a caption too is deleted with its picture (PLAN-image-edit P01.S02, D4)"
#
# The defect: one condition of the picture edit's door is taken out; the named test must go red for its own reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAFigureThatHoldsMoreIsRefusedAndSaysSo"
EXPECT="its element holds a second content item: listed"
