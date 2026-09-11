# docs/red-proofs.md, tier 1: "a new authoring door ships without being asked where its language
# comes from" (PLAN-accessibility.md P03.S02, v1.129.33)
#
# The defect: a seventh function that turns something into a PDF, added beside the six that exist,
# and nobody asked what language its output is in. It ships a document with no /Lang and no record
# of whether that was a decision or an oversight — and from outside, those two look identical.
#
# This is the slice's own acceptance clause, which reads "a new authoring door with none turns the
# guard red, proved by adding one". This IS that proof, recorded so it stays one.
TIER="tier 1 — go test"
PROVE="go test . -run TestEveryAuthoringDoorSaysWhereItsLanguageComesFrom"
EXPECT="authoring door with no language classification"
