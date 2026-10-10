# docs/red-proofs.md, tier 1: "A promotion with no inline tag to number is applied (ADR-126)"
#
# The defect: the no-op refusal is dropped, so a write that changes nothing is recorded as a change and an undo step.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAPromotionIsRefusedWhereThereIsNothingToPromote"
EXPECT="a second promotion: err = <nil>"
