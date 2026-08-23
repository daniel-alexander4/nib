# docs/red-proofs.md, tier 1: "a thirty-day arm polls the DHT at race rates" (/pending 256, v1.117.123)
#
# The defect: a flat 5 s fetch cadence, sized for a 300 s race, applied to the receive arm's
# MaxCeremonyLife window. A "poll" here is a full iterative DHT traversal fanned out to the routing
# table, not one datagram. lan.go computed exactly this harm for the LAN announcer (5.2M multicast
# datagrams) and capped the announcer at five minutes; the DHT half never got the same treatment.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheRendezvousCadenceStepsDownButNeverStops -count=1"
EXPECT="nothing steps down"
