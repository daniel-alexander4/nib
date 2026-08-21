# docs/red-proofs.md, tier 1: "The feeder's ctx.Done() arm removed" (P05.S03 remediation)
#
# The defect: raceCandidates reads its input with `for c := range in`, which ends only when
# the CALLER closes the channel. The racer cancels on a win but cannot close `in`, so against
# a trickle source the feeder blocks on the receive forever, its deferred close(results) never
# runs, and the drain goroutine started on the win path never returns either.
#
# Why nothing else catches it: dialAny closes the channel it builds, so every test that goes
# through it leaves the loop the ordinary way; and the one test that does drive an open
# channel drives the LOSS path, where the deadline ends the race regardless.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheFeedStopsWhenTheRaceIsWon"
EXPECT="the feed goroutine is still running"
