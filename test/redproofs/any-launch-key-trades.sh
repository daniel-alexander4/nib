# docs/red-proofs.md, tier 1: "a key nobody minted trades for the token"
# (/pending 685)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestALaunchKeyTradesOnce -count=1"
EXPECT="second trade of the same key = 200, want 403"
