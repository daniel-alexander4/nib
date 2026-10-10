# docs/red-proofs.md, tier 1: "Removing a tree takes each opener and leaves its EMC (ADR-120, v1.203.0)"
#
# The defect: every BDC is removed and its EMC is left, so the page closes marked content it never opened.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestRemovingTagsTakesOnlyTheBrackets"
EXPECT="the page now draws"
