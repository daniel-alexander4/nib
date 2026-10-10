# docs/red-proofs.md, tier 1: "Removing a tree leaves an annotation's /StructParent (ADR-120, v1.203.0)"
#
# The defect: the tree goes and a form field's widget keeps the /StructParent that named a row of it.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestRemovingTheTreeLeavesAnUntaggedDocumentThatDrawsTheSame"
EXPECT="still name a /ParentTree row"
