# docs/red-proofs.md, tier 1: "The announced transport ignored" (ADR-010)
#
# The defect: a discovered candidate drops the transport the announcement carried, so the
# dialer falls back to the caller's own request and a QUIC-armed peer is dialled over TCP —
# a connection refused at a UDP port, surfaced to the user as an unreachable peer.
#
# The same defect is provable at TIER 4 (`./build/pairrepro.sh --lan` fails its QUIC run),
# which is where it was actually caught. This row is the tier-1 replay, which is seconds
# rather than minutes and needs no network namespace.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAQUICArmedPeerIsDialledOverQUIC"
EXPECT="the candidate's transport is"
