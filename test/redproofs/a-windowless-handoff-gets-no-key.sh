# docs/red-proofs.md, tier 1: "a hand-off with no document answers "window" and no launch key, so the window it opens has no way to get the token"
# (/pending 685)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAWindowlessHandoffGetsAKeyForThisInstance -count=1"
EXPECT="a hand-off with no document = 200/\"window\" launch=false"
