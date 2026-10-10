# docs/red-proofs.md, tier 1: "A deleted tag's annotation keeps a /ParentTree entry naming the deleted tag (ADR-124)"
#
# The defect: an OBJR kid is moved to the parent and the annotation's `/StructParent` entry is left naming an element nothing lists.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestEverySlotThatNamedTheDeletedElementNamesItsParent"
EXPECT="the annotation's entry names"
