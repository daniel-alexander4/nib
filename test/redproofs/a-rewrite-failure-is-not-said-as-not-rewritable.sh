# docs/red-proofs.md, tier 1: "A document that does not survive the rewrite is refused without its cause (PLAN-image-edit P01.S02)"
#
# The defect: one condition of the picture edit's door is taken out; the named test must go red for its own reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestADocumentThatDoesNotSurviveARewriteIsNotRewritable"
EXPECT="want a not-rewritable refusal"
