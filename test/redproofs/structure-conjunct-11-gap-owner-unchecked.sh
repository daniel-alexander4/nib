# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — (11) the gap belongs to THIS object
#
# The defect: (11) the gap belongs to THIS object is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestEachStructureConjunctRefusesItsOwnFixture"
EXPECT="want conjunct 11 cause"
