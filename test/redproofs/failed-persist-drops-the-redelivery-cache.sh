# docs/red-proofs.md, tier 1: "a failed durable write costs the re-delivery cache" (/pending 343, v1.117.300)
#
# The in-memory entry is written only after the mirror write succeeds, so a disk failure leaves the
# cache empty. Plausible-looking — "don't record what we couldn't store" — and it is the second
# signature D24 forbids: the initiator's reconnect misses the cache, falls through to `Contribute`,
# and stacks a second block from one identity onto one document.
#
# The test double in `internal/p2p/redeliver_test.go` already caches on failure and says why; this
# row is that clause asserted against the PRODUCTION implementation, which is where it binds.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheContributionReachesDiskInsideStore -count=1"
EXPECT="dropped the in-memory re-delivery entry"
