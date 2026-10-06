# docs/red-proofs.md, tier 1: "Each of several launches opens its own window (ADR-086, v1.184.0)"
#
# The defect: a launch that was told to open a window is not remembered, so the launches behind it —
# one per file selected in a file manager — each open one too, before the first has connected.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -count=1 -run TestALaunchWaitsForAWindowAlreadyOnItsWay"
EXPECT="one window per file"
