# docs/red-proofs.md, tier 1: "the arm publishes once and is un-findable for 29 days" (/pending 256,
# v1.117.123)
#
# The defect: publishWhenSlow published ONCE. The record lives candidateLife() = 8 minutes and the arm
# lives MaxCeremonyLife = 30 days, so a peer dialling at hour three finds nothing — and D19 tells them
# the other side "hasn't started their ceremony yet" about a machine that has been listening for hours.
# Extending the record instead is impossible: MaxCandidateLife is a READER-side ceiling of one hour that
# every peer enforces, so republishing is the only mechanism there is.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnArmRepublishesForAsLongAsItIsArmed -count=1"
EXPECT="leaving the arm un-findable"
