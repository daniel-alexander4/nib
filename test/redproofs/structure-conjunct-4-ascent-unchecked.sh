# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — (4) starts strictly ascend
#
# The defect: (4) starts strictly ascend is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestEachStructureConjunctRefusesItsOwnFixture"
EXPECT="want conjunct 4 cause"
