# docs/red-proofs.md, tier 1: "a delivery round stops when the request that started it goes away"
# (/pending 355, v1.117.345)
#
# The defect: the shipped one. `raceWithRendezvous` built its deadline from `context.Background()`,
# so the `r.Context()` that `handleCeremonyDeliver` threads down governed nothing — a client that
# disconnected mid-round left it running to `connectDeadline`, 300 s per remaining party, up to
# about forty minutes at nine parties, for a response nobody would read.
#
# The window is what makes the assertion real: the context is alive when the round starts and dies
# during the first leg, so the loop's own short-circuit cannot be what ends it. Red at the test's
# 45 s ceiling against a 2 s green.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestARoundStopsWhenTheRequestThatStartedItGoesAway -count=1"
EXPECT="still running 45 s after the context that started it was cancelled"
