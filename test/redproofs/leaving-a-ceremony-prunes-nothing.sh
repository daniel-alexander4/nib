# docs/red-proofs.md, tier 1: "leaving a ceremony prunes nothing" (P05.S01, v1.128.10)
#
# The defect: the route answers 200 and removes nothing. Since D14 the arm is raised by a sweep and
# renewed at EVERY unlock, so a leave that does not prune is not a weak fix — it is no fix: the
# next unlock arms again and the user has no way to tell, because nothing is on screen either way.
#
# The check drives the arm, leaves, and runs the sweep AGAIN, which is what a restart performs. A
# check made only immediately after leaving would pass against this defect.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestLeavingStopsTheArmAndKeepsItStopped -count=1"
EXPECT="armed for a ceremony this machine had left"
