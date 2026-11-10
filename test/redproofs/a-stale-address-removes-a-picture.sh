# docs/red-proofs.md, tier 1: "An address whose digest no longer names its picture removes one anyway (PLAN-image-edit P01.S02, D2)"
#
# The defect: one condition of the picture edit's door is taken out; the named test must go red for its own reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAStaleAddressIsRefusedAndNothingIsWritten"
EXPECT="a wrong digest: bytes were returned"
