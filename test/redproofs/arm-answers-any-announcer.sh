# docs/red-proofs.md, tier 1: "an armed ceremony party answers any announcer" (P07.S05c, v1.117.183)
#
# The defect: `answerLoop` re-announces on ANY sighting instead of only the peer its arm was raised
# for. Two harms, and the second is the one that scales: L1 says an arm may act only on a name it
# already holds, and two armed parties that have pinned each other answer each other's answers
# forever — a standing beacon between two machines with nothing to say, which is the exact cost
# `lanAnnounceWindow` exists to refuse.
#
# **The first version of this guard could not catch it, and that is why the seam carries a
# candidate.** `answer` took no arguments, so the test could count answers and not identify them —
# and a stranger's sighting followed by a later one produces the same COUNT as the peer's sighting
# followed by a later one. Deleting the gate left every assertion green. `answerLoop` now hands the
# resolved candidate to the answer callback, which production ignores and the test asserts on.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheArmAnswersItsOwnPeerAndNobodyElse -count=1"
EXPECT="want this arm's own peer"
