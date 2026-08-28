# docs/red-proofs.md, tier 1: "The punch ceiling moves into the tuning block" (P07.S09a, D33, v1.117.223)
#
# The defect: `MaxCandidates` — D33's candidate cap, `N` — is mirrored into `clocks.go`, the block
# whose own premise is that its values are tuning.
#
# A SINGLE-FILE mutation, and that constraint improved the row. The first version moved
# `punchBudgetPerSide` between two files, which `verify_test.go` refuses: "a red proof plants ONE
# defect, and a patch with more is the bare-`git diff` mistake that once swept four rows into each
# other while every one of them still replayed green". Mirroring the cap is both smaller and more
# realistic — it is what somebody does for convenience, not what a refactor does by accident.
#
# D33 as amended (Dan, 2026-08-19) makes the candidate cap and the punch ceiling LAW on D6's
# reasoning: an attacker supplies the candidates, so a bound an operator may raise is not a bound.
# Nothing observable changes when the constant moves, which is why the decision asks for a
# source-level guard and says so twice.
#
# Nothing observable changes when a figure is mirrored — both copies read 8 — which is precisely
# why the decision asks for a source-level guard and says so twice.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestNeitherLawFigureIsReachableFromTheTunableBlock"
EXPECT="That figure is LAW"
