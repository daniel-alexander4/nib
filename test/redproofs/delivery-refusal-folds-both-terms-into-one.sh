# docs/red-proofs.md, tier 1: "the refusal states one total instead of two terms" (P08.S05b, v1.117.292)
#
# The refusal is the only place a convener learns how much time to add. Folding the delivery
# reservation into a single "N hops need X" figure makes that sentence an arithmetic lie: X is not
# hop time, and a convener who divides it by N to size a hop gets a number twice too large.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestTheRefusalNamesBOTHTermsRatherThanOneTotal -count=1"
EXPECT="does not contain"
