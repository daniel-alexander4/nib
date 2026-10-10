# docs/red-proofs.md, tier 1: "A top-level tag that holds content itself is deleted (ADR-124)"
#
# The defect: the refusal is dropped, so marked content lands in the root's `/K`, which may hold only structure elements (ISO 32000-1 Table 322).
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestADeleteThatWouldBreakTheTreeIsRefused"
EXPECT="a top-level element that holds content: err ="
