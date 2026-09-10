# docs/red-proofs.md, tier 1: "every request field a handler reads is one some client sends"
# (/pending 447, v1.128.96)
#
# The defect this reproduces is the item's founding one, verbatim. `internal/server/cosign.go` said
# that adding an `invitation` parameter "would be a field no client fills, which is the shape this
# repo's reader scans exist to refuse" — and NO SCAN REFUSED IT. `observables_test.go` cannot see a
# multipart form field, and for JSON shapes its matcher passed on coincidental name collisions.
# This patch does the thing that comment said was already policed.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryRequestFieldAHandlerReadsIsOneSomeClientSends -count=1"
EXPECT="read by a handler and sent by no client"
