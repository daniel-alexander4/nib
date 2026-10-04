# docs/red-proofs.md, tier 1: "/api/status carries the token again — any caller holding the session reads it back, and the defect /pending 685 closed was exactly a status read that handed it out"
# (/pending 685, v1.167.0)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestStatusNeverCarriesTheToken -count=1"
EXPECT="GET /api/status carries the token"
