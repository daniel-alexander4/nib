# docs/red-proofs.md, tier 1: "a missing delivery budget is defaulted rather than refused" (P08.S05b, v1.117.292)
#
# A duration whose zero value means "everything fits" is the rule switched off — the reason
# HopBudget refuses its own zero. This row exists because a mutation found the hole: the first cut
# of the slice deleted this guard and the ENTIRE suite stayed green, since every fixture supplies a
# budget and nothing asked what happens when a caller does not. A two-arm rule covered on one arm.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestConveneRefusesEachThingByName -count=1"
EXPECT="no_delivery_budget"
