# docs/red-proofs.md, tier 1: "A role map edit remaps a standard structure type (ADR-127)"
#
# The defect: a standard type is accepted as the name of a mapping, which is the remap ua1 7.1 t7 fails.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestARoleMapEditThatCannotBeAppliedIsRefused"
EXPECT="a standard type as the name: err = <nil>"
