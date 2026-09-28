# docs/red-proofs.md, tier 1: "the field rebuild decides each field once" (/pending 689)
#
# The defect: `keepFields` rebuilds the AcroForm field tree with no memo, so a /Kids array naming one
# child k times is walked k^depth times — measured, 1,596 bytes (k=4, 12 levels) cost `RemovePages`
# 1.15 s and k=8 did not finish in 300 s. The check counts predicate calls, never wall-clock.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -run TestKeepFieldsDecidesEachFieldOnce -count=1"
EXPECT="the rebuild pays per path"
