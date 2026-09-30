# docs/red-proofs.md, tier 1: P01.S04 of PLAN-returned-document.md — (8) the bytes arm
#
# The defect: (8) the bytes arm is no longer checked in internal/sign/revisions.go.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run TestEachStructureConjunctRefusesItsOwnFixture"
EXPECT="(8) gap at a same-length hex token: conjunct"
