# docs/red-proofs.md, tier 1: "A proposed header cell is committed with no /Scope (ADR-121, v1.204.0)"
#
# The defect: the commit writes Table, TR, TH and TD and never scopes a header cell, which is ua1 7.5 t1.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestACommittedTableReadsBackNested"
EXPECT="the committed table reads"
