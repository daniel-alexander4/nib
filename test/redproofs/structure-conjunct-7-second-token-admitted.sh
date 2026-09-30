# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — (7) the token arm: one hex token fills the gap
#
# The defect: (7) the token arm: one hex token fills the gap is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestEachStructureConjunctRefusesItsOwnFixture"
EXPECT="want conjunct 7 cause"
