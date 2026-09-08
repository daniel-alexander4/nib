# docs/red-proofs.md, tier 1: "the arm teardown matches every ceremony" (/pending 378, v1.128.25)
#
# The defect: `stopListeningFor` drops its id comparison and tears down every ceremony arm. The
# mirror image of the row above, and the one a fix reaching for `disarm()` would have shipped: a
# user who leaves one proceeding loses the arm for another they are still waiting on, and nothing
# tells them. Leaving is named at one ceremony; the teardown must be too.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestLeavingOneCeremonyLeavesAnotherAlone -count=1"
EXPECT="tore down the arm this machine holds for a DIFFERENT one"
