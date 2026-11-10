# docs/red-proofs.md, tier 1: "A signed document has a picture removed (PLAN-image-edit P01.S02, D5)"
#
# The defect: one condition of the picture edit's door is taken out; the named test must go red for its own reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/objedit/ -count=1 -run TestASignedDocumentIsRefusedBeforeAnyRead"
EXPECT="bytes were returned for a signed document"
