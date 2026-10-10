# docs/red-proofs.md, tier 1: "A region brackets, on the page, content a form XObject draws (ADR-125)"
#
# The defect: the in-form refusal is dropped, so spans read from a form's stream are written into the page's own.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestARegionThatCannotBeWrittenIsRefused"
EXPECT="bare content a form draws"
