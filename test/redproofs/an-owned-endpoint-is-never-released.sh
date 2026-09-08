# docs/red-proofs.md, tier 1: "an owned endpoint is never released" (/pending 376, v1.128.28)
#
# The defect: `setupSharedEndpoint` marks the endpoint it just OPENED as borrowed, so `close()`
# returns early and nothing is ever torn down — a socket, a DHT server and a port-mapping lease
# leaked per arm for the life of the process. The mirror image of the row above, and the reason the
# flag is set where ownership is decided rather than defaulted.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnOwnedEndpointIsStillTornDown -count=1"
EXPECT="marked as borrowing it"
