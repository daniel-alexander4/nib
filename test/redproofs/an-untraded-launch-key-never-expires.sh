# docs/red-proofs.md, tier 1: "the launch key's TTL is ignored — a key minted for a window that never opened trades forever"
# (/pending 685)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnUntradedKeyExpires -count=1"
EXPECT="an expired key traded: 200, want 403"
