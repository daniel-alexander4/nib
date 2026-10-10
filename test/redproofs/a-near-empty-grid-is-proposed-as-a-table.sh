# docs/red-proofs.md, tier 1: "A near-empty ruled grid is proposed as a table (ADR-123, v1.205.1)"
#
# The defect: the quarter-filled rule falls back to "holds any text at all", so a worksheet's ruling with a
# word here and there is proposed as a table of empty cells.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestAGridWithFewerThanAQuarterOfItsCellsFilledIsNotATable"
EXPECT="two cells of nine: the grid changed the proposal"
