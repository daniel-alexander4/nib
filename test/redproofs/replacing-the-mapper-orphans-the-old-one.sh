# docs/red-proofs.md, tier 1: "a republish orphans the mapper it replaces" (v1.117.130, from grilling
# /pending 262)
#
# The defect, and this week's own change made it reachable: v1.117.123 turned the one-shot publish into a
# republish loop, so every cycle builds a fresh mapper — and setPortMap overwrote the field without
# closing what was there. Refresh goroutine still running, router mapping still installed, nothing
# holding a handle to either. Not a corner: the republish period is 240 s inside a 300 s connect deadline.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestReplacingTheStoredMapperClosesTheOldOne -count=1"
EXPECT="left the old one open"
