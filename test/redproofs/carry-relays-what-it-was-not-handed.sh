# docs/red-proofs.md, tier 1: "the carrier relays a document it did not hand over" (P07.S05, v1.117.176)
#
# The defect: `Carry` stops checking that what came back is what went out plus a trailing update.
# A hostile hop can then return **any** document these identities ever produced, and the carrier
# relays it to the next party as this ceremony's baton.
#
# `Initiate` has this check and states the reasoning: the signer is strictly append-only, so a
# legitimate contribution is always the sent bytes plus an incremental update.
#
# Driven through the REAL verb against a hostile receiver, not through a helper — an earlier draft
# ran the checks in a copy beside the test, which is a fixture asserting itself: the mutation that
# matters is deleting the check from `Carry`, and a helper carrying its own copy stays green
# against exactly that.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestCarryRefusesAHostileHop -count=1"
EXPECT="a different document"
