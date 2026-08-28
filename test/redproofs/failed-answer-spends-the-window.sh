# docs/red-proofs.md, tier 1: "an answer that never happened silences the arm" (P07.S05c, v1.117.183)
#
# The defect: the rate-limit clock is stamped whether or not the announcement actually went out.
# `startAnnouncing` refuses by name on a loopback bind and returns an error when there is no usable
# interface — both ordinary, both non-fatal to the arm — so a party that has said NOTHING is then
# treated as having just spoken and ignores the next sighting.
#
# It is the shape a "did I do the thing?" check exists for: the failure is a SUCCESS PATH in
# disguise, because everything about the arm still looks healthy. The fix is one branch, and the
# reason it is a red proof rather than a comment is that nothing else in the tree would notice the
# branch being flattened — the mechanism keeps working on every machine where announcing succeeds.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAFailedAnswerDoesNotSpendTheWindow -count=1"
EXPECT="a failed answer spent the rate-limit window"
