# docs/red-proofs.md, tier 1: "An ignored inline image is bracketed at its own span, which pdfcpu refuses (ADR-122)"
#
# The defect: the artifact about an inline image closes straight after its EI, so pdfcpu reads a corrupt image and the page cannot be committed.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAnIgnoredInlineImageCommitsAsAnArtifact"
EXPECT="corrupt BI expression"
