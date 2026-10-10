# docs/red-proofs.md, tier 1: "A promotion takes a /ParentTree slot that names another tag (ADR-126)"
#
# The defect: a slot that already names an element is written over with the promoted one, so a defect the tree had becomes a second owner the edit made.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestASlotThatNamesAnotherElementIsLeftAndDoesNotRefuseThePromotion"
EXPECT="slot 1 named element 20 and still does"
