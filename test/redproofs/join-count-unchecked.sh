# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — the join's shift detection: the count check
#
# The defect: the join's shift detection: the count check is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestTheJoinRefusesEveryDisagreement"
EXPECT="want the count disagreement"
