# docs/red-proofs.md, tier 1: "an observed-permanent lease written as a zero lifetime" (/pending 260,
# v1.117.127)
#
# The defect: the naive fix. IGD reports a permanent mapping as NewLeaseDuration 0, and `refreshAfter`
# already reads a zero lifetime as its OPPOSITE — "no grant was reported, use the floor". Writing the
# observed 0 into LifetimeSec therefore refreshes a mapping that never expires every fifteen seconds,
# forever: four times more work in the one case that needs least. One uint32 cannot carry "unknown" and
# "infinite" for a reader that treats them as opposites, so LeasePermanent is its own field.
TIER="tier 1 — go test"
PROVE="go test ./internal/portmap/ -run TestObserveLease -count=1"
EXPECT="a mapping that never expires"
