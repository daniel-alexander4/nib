# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — (1), the length arm: at least two pairs
#
# The defect: that arm of conjunct (1) is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestEachStructureConjunctRefusesItsOwnFixture"
EXPECT="(1) one pair: conjunct"
