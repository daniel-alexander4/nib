# docs/red-proofs.md, tier 1: "an arm is torn down mid-session" (P02 close, v1.128.14)
#
# The defect: the in-flight condition goes, so a policy arm is displaced while a consent request or
# a spoken check is on screen. A peer is mid-exchange and a person is looking at four words; tearing
# that down answers on their behalf, which is the failure `disarmIf` records for the arm-window
# timer.
#
# **This row exists because a probe showed the condition decided nothing.** The two tests written
# alongside the fix both drive a policy arm with nothing in flight, so removing the guard left them
# green. The case that reaches it parks a spoken check directly — the state under test, not the
# route to it.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAPolicyArmWithSomethingOnScreenIsNotDisplaced -count=1"
EXPECT="displaced a policy arm with the spoken check on screen"
