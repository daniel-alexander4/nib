# docs/red-proofs.md, tier 1: "A kept figure is bracketed as an artifact as well as a Figure (ADR-122)"
#
# The defect: the artifact pass no longer leaves out the drawing a Figure took, so a described picture is also called decoration.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAKeptFigureIsContentAndAnIgnoredOneAnArtifact"
EXPECT="is not inside its own sequence"
