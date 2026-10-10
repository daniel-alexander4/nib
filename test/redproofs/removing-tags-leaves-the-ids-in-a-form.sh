# docs/red-proofs.md, tier 1: "Removing a tree leaves the marked-content ids inside a form XObject (ADR-120, v1.203.0)"
#
# The defect: the removal rewrites each page's own content and skips the form XObjects the pages draw, so a 2-up or any
# document tagged inside a form keeps ids with no tree — the state the commit writer refuses.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestRemovingTheTreeLeavesAnUntaggedDocumentThatDrawsTheSame"
EXPECT="still carry an id"
