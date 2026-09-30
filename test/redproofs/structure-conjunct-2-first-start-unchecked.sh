# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — (2) the first start is 0
#
# The defect: (2) the first start is 0 is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestEachStructureConjunctRefusesItsOwnFixture"
EXPECT="want conjunct 2 cause"
