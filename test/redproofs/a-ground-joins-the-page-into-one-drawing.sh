# docs/red-proofs.md, tier 1: "A ground painted under the page joins everything on it into one drawing (ADR-125)"
#
# The defect: paths are joined through a plain rectangle that holds them whole, so a page with a ground lists one drawing and a chart on it cannot be ticked alone.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestTheUntaggedReaderSaysWhatAFormDrawsAndWhatIsDecoration"
EXPECT="a ground, a grid, a curve and a lone rule list as"
