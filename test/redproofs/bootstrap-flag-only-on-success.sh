# docs/red-proofs.md, tier 1: "bootstrapDone is set only when the bootstrap SUCCEEDS"
# (P07.S05d, v1.117.203)
#
# The defect: the flag gating D19's arm-side live diagnosis is stored only on the success path.
# It reads as the careful choice and it inverts the feature. Until the bootstrap has had its
# chance, zero DHT responses means "still warming up" rather than "unreachable", so the flag
# suppresses a scary false alarm on a healthy machine — which means a flag set only on success
# leaves the machine whose network is ACTUALLY dead as the one that never gets told, because the
# diagnosis it needs is gated on the very thing that failed.
#
# Latent until P07.S05d: while the bootstrap was eager, the store sat beside it on a path that
# ran unconditionally. Moving it inside the door is what makes the success/failure distinction
# reachable at all.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheBootstrapDoorSetsItsFlagEvenWhenTheBootstrapFAILS"
EXPECT="gated forever on the machine that most needs it"
