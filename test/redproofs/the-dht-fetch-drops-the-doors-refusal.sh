# docs/red-proofs.md, tier 1: "the fetch calls the switch's door and throws its refusal away — the candidate feed reaches the DHT with remote rendezvous OFF"
# (/pending 690, ADR-011)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheCandidateFeedHonoursTheRendezvousSwitch -count=1"
EXPECT="the candidate feed sent 1 datagram(s) to a DHT node"
