# docs/red-proofs.md, tier 1: "the carrier relays a contribution out of order" (P07.S05, v1.117.176)
#
# The defect: `Carry` stops running L3 over the RETURNED document. It keeps the byte prefix, and
# that is exactly why this row exists separately: **the prefix says the bytes grew from mine and
# says nothing at all about who signed the part that grew.** Measured — with the prefix check alone
# a reply extended by the WRONG party is accepted.
#
# The carrier is not the contributor, so S03's door — which answers the contributor's question —
# is passed through by nobody on this path unless the carrier asks it here.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestCarryRefusesAHostileHop -count=1"
EXPECT="extended_by_the_wrong_party"
