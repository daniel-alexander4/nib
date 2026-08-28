# docs/red-proofs.md, tier 1: "The signing roster drops what the block needs" (P07.S07a, v1.117.216)
#
# The defect: `l3Roster` goes back to copying `Fingerprint` and `Signs` out of the invitation and
# dropping `Label` and `Capacity`. This is the ORIGINAL defect — it is how `Party.Label` came to
# sit inside the signed commitment for three phases with no display reader anywhere.
#
# **The row exists because `internal/p2p` stays GREEN under it, and that is the point.** Every
# block test in that package builds its own roster, so each supplies the very thing production
# fails to supply — a fixture satisfying the condition the code under test does not establish,
# which is this repo's recurring vacuous green. The arm that catches it has to be on the
# conversion itself, in the package that performs it.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheInvitationsLabelsAndCapacitiesReachTheSigningRoster"
EXPECT="a label dropped here is a block that says"
