# docs/red-proofs.md, tier 1: "a sweep trigger launches its own goroutine" (/pending 515)
#
# ADR-009's second half — *"the guard asserts routing through the door, not the text each site
# prints"*. A lock inside `runCeremonySweep` is worth nothing if a trigger goes around it, and the
# concurrency counter one row up cannot tell: it drives the door directly, so it stays green while
# `adoptVault` runs the close-out, both re-arms and the seed beside a sweep already doing them.
#
# The mutation is the shape this started as: `adoptVault`'s `s.runCeremonySweep("delivery re-arm",
# func() { … })` back to a bare `go func() { … }()`, which is what it was until /pending 515.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryCeremonySweepGoesThroughTheOneDoor -count=1"
EXPECT="no longer routes its sweep through runCeremonySweep"
