# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — (5) the count arm: exactly one gap
#
# The defect: (5) the count arm: exactly one gap is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestEachStructureConjunctRefusesItsOwnFixture"
EXPECT="(5) two gaps: conjunct"
