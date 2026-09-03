# docs/red-proofs.md, tier 1: "the close-out teardown reaches three of the four ceremony stores"
# (P08.S06, D29, v1.117.330)
#
# P08.S06's own scope said the close-out was "three stores, not two" — pins, secrets and the
# mirror — and it is FOUR. The invitee-side stored invitation is the fourth, and it carries the
# ceremony secret. `unconvene` was written with three and P08.S01 added the fourth with the reason
# this proof preserves: *"almost always is not a reason for a teardown to reach one of two stores,
# and a convener that also accepted an invitation to its own ceremony is exactly the case nobody
# would think to test."* The mutation re-opens it one door over, where a plan bullet written before
# that fix would have put it back.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheCloseOutDoorReachesEveryCeremonyScopedStore -count=1"
EXPECT="stored invitation survived the close-out"
