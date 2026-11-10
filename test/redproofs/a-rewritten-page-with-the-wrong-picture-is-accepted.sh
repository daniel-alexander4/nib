# docs/red-proofs.md, tier 1: "The after-check accepts a written page missing the wrong picture (PLAN-image-edit P01.S02, D2's pin)"
#
# The defect: one condition of the picture edit's door is taken out; the named test must go red for its own reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestTheWrittenPageIsHeldToWhatTheRemovalLeaves"
EXPECT="want a rewrite-differs refusal, got <nil>"
