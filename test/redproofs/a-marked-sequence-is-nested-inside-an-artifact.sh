# docs/red-proofs.md, tier 1: "A region writes a marked sequence inside the artifact it took (ADR-125)"
#
# The defect: the artifact's opener is left and a marked opener is written inside it, so the page calls the content decoration and the tree calls it content.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestARegionMakesAWholeArtifactSequenceContent"
EXPECT="the artifact's opener was not rewritten in place"
