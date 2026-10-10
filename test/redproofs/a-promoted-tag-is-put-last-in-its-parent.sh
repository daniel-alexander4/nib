# docs/red-proofs.md, tier 1: "A promoted tag is listed last in its parent, not where it was written (ADR-126)"
#
# The defect: the reference is appended to /K instead of taking the inline dictionary's place, so the reading order changes.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAPromotedElementTakesItsPlaceInItsParentsKByReference"
EXPECT="in that order"
