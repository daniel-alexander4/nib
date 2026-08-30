# docs/red-proofs.md, tier 1: "a frozen refusal code is renumbered" (/pending 315, v1.117.257)
#
# The defect: `refusePrefixMismatch` moves from 3 to 30. A wire code is a value two builds must
# agree on and the const block's own doc says "Append only, and never renumber" — but a renumber
# keeps BOTH switches symmetric, keeps both hand-written literal slices valid, and leaves every
# other test in the package green. A peer on the shipped build then reads 3 as this refusal and 30
# as something it has no sentinel for.
#
# This is the one arm of the guard that is deliberately NOT derived. The frozen table is
# hand-written because these are wire values: a number a future build may change was never a wire
# value at all, so the table is a record and adding a line to it is the act of freezing.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheRefusalEnumerationIsDerivedFromSource -count=1"
EXPECT="and the wire pin says"
