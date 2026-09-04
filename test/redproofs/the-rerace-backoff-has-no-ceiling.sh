# docs/red-proofs.md, tier 1: "the re-race backoff is capped" (/pending 369, v1.117.353)
#
# The defect: the backoff doubles without a ceiling.
#
# A delay that grows without bound stops pacing and starts refusing: an arm waiting an hour between
# attempts is not listening for its peer, it is asleep through the ceremony. The ceiling is also
# what makes the RATE knowable without measuring it — once warmed, the arm cannot re-enter more
# often than `reraceCap` whatever the peer does, which is the question /pending 369 was going to
# answer with a tier-4 flapping-peer harness this repo does not have.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheReraceIsPacedAndBounded"
EXPECT="want the ceiling"
