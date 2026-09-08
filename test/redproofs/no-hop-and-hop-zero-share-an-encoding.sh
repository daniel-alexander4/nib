# docs/red-proofs.md, tier 1: "no hop and hop zero share an encoding" (/pending 385, v1.128.30)
#
# The defect: `hopNoneWire` collapses onto 0. Hop 0 is the CONVENER'S OWN INDEX — a real, ordinary
# arm — so every manual or LAN receive arm would advertise itself as that hop and a ceremony dial
# would match it. This is why the sentinel is not the zero value, unlike `TransportTCP`.
TIER="tier 1 — go test"
PROVE="go test ./internal/discovery/ -run TestHopZeroIsNotTheSameAsNoHop -count=1"
EXPECT="encode identically"
