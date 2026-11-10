# docs/red-proofs.md, tier 1: "A Figure whose sequence also paints a shading is deleted with its picture (PLAN-image-edit P01.S02, code-review 2026-11-09)"
#
# The defect: one condition of the picture edit's door is taken out; the named test must go red for its own reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAShadingInsideTheFiguresSequenceKeepsItsElement"
EXPECT="want tagged-shared"
