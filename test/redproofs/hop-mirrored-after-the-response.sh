# docs/red-proofs.md, tier 1: "a hop is mirrored after its response returns" (P07.S05a, v1.117.178)
#
# The defect: `handleSessionInitiate` writes the JSON response and THEN calls `mirrorHop`. Both
# calls are present and the mirror is written on every ordinary run, so no behavioural test can
# see it — the failure needs a crash in the window between them, and what it costs is a user who
# was told their signature is safe and a machine with no copy of it.
#
# C22's clause is "before the HTTP response returns", so the ORDER is the criterion and not an
# implementation detail. That is why the guard is structural: the property is about which
# statement runs first, and there is no observation of a completed request that distinguishes them.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestBothSidesOfAHopMirrorIt -count=1"
EXPECT="mirrored AFTER the response returns"
