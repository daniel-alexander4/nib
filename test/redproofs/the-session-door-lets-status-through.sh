# docs/red-proofs.md, tier 1: "requireSession lets a stranger read /api/status — the read that listed SSH key paths and ran first-run setup for any local process"
# (/pending 685, ADR-054)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestStatusNeverCarriesTheToken -count=1"
EXPECT="GET /api/status to a stranger = 200"
