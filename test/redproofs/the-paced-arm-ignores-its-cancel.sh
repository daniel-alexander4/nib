# docs/red-proofs.md, tier 1: "pacing honours a teardown" (/pending 369, v1.117.353)
#
# The defect: `sleepOrDone` waits the timer unconditionally instead of racing it against the context.
#
# A bare sleep in a retry loop is a cancel the arm does not honour — at `reraceCap` that is two
# seconds per attempt of a goroutine outliving the session that owned it, on every disarm and every
# shutdown. The pacing added for /pending 369 must not be paid for with a teardown that hangs.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestPacingIsAbandonedTheMomentTheArmIsCancelled"
EXPECT="cancelled arm completed its wait"
