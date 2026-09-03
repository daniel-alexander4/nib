# docs/red-proofs.md, tier 1: "the close-out grace is a literal that agrees with the ceiling today"
# (P08.S06, the maxCandidatesPerSource rule, v1.117.330)
#
# `72 * time.Hour` and `ceremony.MaxCeremonyLife / 10` are the same number and behave identically,
# so no behavioural test can tell them apart — `lawplacement_test.go` says exactly this about D33
# and it is why that guard reads source rather than running anything. Move the 30-day ceiling and
# the literal stays behind, closing out ceremonies the record still considers live. A comment
# claiming the two agree is not a mechanism; `NominalBlockRect` is this repo's standing example of
# what a hand-copy plus a comment costs.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheGraceIsDerivedFromTheCeremonyCeilingAndNotHandCopied -count=1"
EXPECT="none is ceremony.MaxCeremonyLife"
