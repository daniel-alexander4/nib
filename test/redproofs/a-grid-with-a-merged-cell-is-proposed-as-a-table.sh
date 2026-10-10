# docs/red-proofs.md, tier 1: "A ruled grid with a merged cell is proposed as a regular table (ADR-121, v1.204.0)"
#
# The defect: a framed grid with a side missing inside it is read as regular, so a merged cell is proposed as two cells.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestWhatIsNotARegularRuledGridIsNotATable"
EXPECT="a grid with a merged cell: the drawing changed the proposal"
