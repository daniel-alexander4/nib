# docs/red-proofs.md, tier 1: "a DHT verb called outside dhtPublish/dhtFetch — how three roads went around the switch"
# (/pending 690, ADR-009)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheDHTVerbsHaveExactlyOneDoorEach -count=1"
EXPECT="rz.Fetch must be called from dhtFetch and nowhere else"
