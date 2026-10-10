# docs/red-proofs.md, tier 1: "A /ParentTree row written as its own object is replaced by a direct array (ADR-124)"
#
# The defect: `setParentTreeSlot` writes the array over the reference instead of into the row's object, leaving that object behind unreferenced.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestEverySlotThatNamedTheDeletedElementNamesItsParent"
EXPECT="rather than updated where it lives"
