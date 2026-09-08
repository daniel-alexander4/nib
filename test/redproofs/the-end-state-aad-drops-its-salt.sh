# docs/red-proofs.md, tier 1: "the end-state AAD drops its salt" (/pending 380, v1.128.27)
#
# The defect: `terminationAAD` stops binding the salt, so a sealed end state lifted from one target
# opens at every other target sharing the key.
#
# **Recorded because it SURVIVED the first pass** — nothing drove a record against a target it was
# not published at, which is the only thing the salt in the AAD is for.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestAnEndStateDoesNotOpenAtAnotherTarget -count=1"
EXPECT="opened at a target it was not published at"
