# docs/red-proofs.md, tier 1: "the initiate route bypasses the relay door" (P07.S05, v1.117.176)
#
# The defect: `/api/session/initiate` installs its result through `addDoc` again.
#
# **Found by mutation, not by review.** Swapping the call back left every behavioural test green,
# because they drive the door directly and nothing drove the route's installation — the ADR-009
# shape exactly: a rule that holds where it is called and nowhere else.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheInitiateRouteInstallsThroughTheRelayDoor -count=1"
EXPECT="does not install through the relay door"
