# docs/red-proofs.md, tier 1: "The commit writer writes a Figure with a blank description (ADR-122)"
#
# The defect: the writer no longer refuses a Figure whose description is blank, so a caller that skips the review writes one that fails ua1 7.3 t1.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestTheWriterWritesNoFigureWithoutADescription"
EXPECT="was answered <nil>"
