# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — (10) not in an object stream
#
# The defect: (10) not in an object stream is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestEachStructureConjunctRefusesItsOwnFixture"
EXPECT="want conjunct 10 cause"
