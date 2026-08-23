# docs/red-proofs.md, tier 1: "the other door — time.AfterFunc" (/pending 255, v1.117.113)
#
# The defect: an AfterFunc callback with no recover. AfterFunc runs it on its own goroutine, so a
# panic there has no caller to unwind into — the same law as `go`, through a door the package-local
# guard never walked. Two such callbacks existed, one of them closing a channel on the
# untrusted-datagram path, where a double close is a panic.
TIER="tier 1 — go test"
PROVE="go test ./ -run TestEveryDetachedGoroutineIsRecovered -count=1"
EXPECT="hands time.AfterFunc a callback"
