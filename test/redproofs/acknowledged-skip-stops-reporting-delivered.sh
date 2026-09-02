# docs/red-proofs.md, tier 1: "the two kinds of skip become indistinguishable" (P08.S05e, v1.117.322)
#
# `deliveryOutcome.Skipped` carries two meanings since this slice — a party an earlier run already
# reached, and the party that ENDED the proceeding — and its doc says `Delivered` is what tells
# them apart. Nothing checked that claim. With the acknowledged branch no longer reporting
# `Delivered`, both kinds read the same and a re-run's report says a party who has their copy is
# in the same state as one who can never be reached.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheRoundReportsTheEnderSkippedRatherThanDialling -count=1"
EXPECT="already-acknowledged party is reported"
