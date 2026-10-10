# docs/red-proofs.md, tier 1: "Part of content the page marks as one piece of decoration is taken by a region (ADR-125)"
#
# The defect: the partial refusal is dropped, so a region through an artifact sequence rewrites its opener and tags content the region never held.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestARegionMakesAWholeArtifactSequenceContent"
EXPECT="a region over half an artifact sequence was answered"
