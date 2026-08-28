# docs/red-proofs.md, tier 1: "Every party signs in party 1's capacity" (P07.S07a, v1.117.216)
#
# The defect: `StampCommitment` reads `order[0].Capacity` instead of `order[pos].Capacity`, so
# every block claims the first signer's capacity.
#
# Capacity is a claim about a party's AUTHORITY — "as Director of Acme Ltd", "as attorney under a
# power of attorney" — and it is inside the signed commitment. A block that gives a witness the
# principal's capacity is an affirmative false statement about that party's obligation, inside the
# artifact, over their own signature. D20's capacity amendment exists for exactly this.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestACapacityRendersOnlyForThePartyThatHasOne"
EXPECT="the entry is being read by the wrong index"
#
# **This row was REJECTED on its first replay and the fixture is what changed.** The guard gave a
# capacity to one party only, so the wrong-index read handed everyone "" and the sole signal was
# party 1 losing its capacity — `redproof.sh` reported "went red, but not for its own reason",
# which is the third outcome it exists to distinguish from a pass and from a deleted check. Two
# parties now carry different capacities, so reading the wrong entry produces a positive, wrong
# statement about a party's authority rather than an absence.
