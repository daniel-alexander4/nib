# docs/red-proofs.md, tier 1: "A cell may name a header cell of another table (ADR-119, v1.202.0)"
#
# The defect: the same-table check is dropped from a headers edit, so a cell names a TH its own table does not hold.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestACellEditThatDoesNotDescribeTheTableIsRefused"
EXPECT="a header of another table"
