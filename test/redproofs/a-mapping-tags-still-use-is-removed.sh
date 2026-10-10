# docs/red-proofs.md, tier 1: "A mapping is removed while tags are still typed with its name (ADR-127)"
#
# The defect: the in-use refusal is dropped, so tags are left with a type nothing explains — ua1 7.1 t5.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestARoleMapEditThatCannotBeAppliedIsRefused"
EXPECT="removing a mapping two tags use: err = <nil>"
