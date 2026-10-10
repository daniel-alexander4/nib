# docs/red-proofs.md, tier 1: "A region writes a Figure with no description (ADR-125)"
#
# The defect: ADR-122's law is dropped for a figure made by region, so a drawn graphic is tagged as a Figure that fails 7.3 t1.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestARegionThatCannotBeWrittenIsRefused"
EXPECT="a figure with no description"
