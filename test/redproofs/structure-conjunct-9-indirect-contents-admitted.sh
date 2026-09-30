# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — (9) /Contents is direct
#
# The defect: (9) /Contents is direct is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestEachStructureConjunctRefusesItsOwnFixture"
EXPECT="want conjunct 9 cause"
