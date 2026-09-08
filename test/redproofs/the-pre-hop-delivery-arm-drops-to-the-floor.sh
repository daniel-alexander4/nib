# docs/red-proofs.md, tier 1: "the pre-hop delivery arm drops to the floor" (/pending 377, v1.128.25)
#
# The defect: `deliveryWindowFor` stops recognising `LoadAbsent` as the pre-hop party, so their
# delivery arm takes `sessionAcceptTimeout` (five minutes) instead of their hop arm's window — the
# arm closes long before any convener could reach it. Recorded because it is the regression a fifth
# `LoadState` would have caused SILENTLY: nothing else in the tree drives this function's absent
# arm, and the failure is a shorter timeout rather than an error.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestThePreHopPartyStillClassifiesAsAbsent -count=1"
EXPECT="delivery arm gets"
