# docs/red-proofs.md, tier 1: "the publish goes around the switch's door — the round's end state is published to strangers with remote rendezvous OFF"
# (/pending 690, ADR-011)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheRoundsEndStatePublishHonoursTheRendezvousSwitch -count=1"
EXPECT="the round's end-state publish sent 1 datagram(s) to a DHT node"
