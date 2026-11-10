# docs/red-proofs.md, tier 1: "A picture a form XObject draws is edited as the page's (PLAN-image-edit P01.S02, L5)"
#
# The defect: one condition of the picture edit's door is taken out; the named test must go red for its own reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAFormDrawnPictureIsRefused"
EXPECT="want a form-drawn refusal"
