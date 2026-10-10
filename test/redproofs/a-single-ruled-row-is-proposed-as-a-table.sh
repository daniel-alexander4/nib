# docs/red-proofs.md, tier 1: "A single ruled row of boxes is proposed as a table (ADR-121, v1.204.0)"
#
# The defect: the two-row minimum is dropped, so a strip of boxes — a form line — is proposed as a table.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestWhatIsNotARegularRuledGridIsNotATable"
EXPECT="a strip of one row: the drawing changed the proposal"
