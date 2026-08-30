# docs/red-proofs.md, tier 1: "a termination's state is not signed" (P08.S04b, v1.117.285)
#
# The defect: `state` is dropped from the preimage. Every OTHER axis stays valid — the version, the
# convener, the roster commitment, the signature — so a `completed` object flips to `declined` and
# verifies perfectly. Which of two things happened is the entire content of this artifact.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run ATerminationBindsExactlyOneProceeding -count=1"
EXPECT="still verified"
