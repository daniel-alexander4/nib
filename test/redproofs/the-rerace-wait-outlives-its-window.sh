# docs/red-proofs.md, tier 1: "the re-race wait never outlives its window" (/pending 369, v1.117.353)
#
# The defect: the backoff is not clamped to the time remaining.
#
# At the ceiling against a window with 10 ms left the wait is 200x too long: the arm sleeps past
# its own deadline and wakes to discover it has ended. The deadline check and the delay are one
# question through one door precisely so a site cannot take the second and miss the first.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheReraceStopsAtItsDeadlineAndNeverSleepsPastIt"
EXPECT="wake after the arm"
