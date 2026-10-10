# docs/red-proofs.md, tier 1: "A role map key is escaped by hand before pdfcpu escapes it (ADR-127)"
#
# The defect: the name is passed through EncodeName when stored, so a name with a space is written as #2320 and no longer matches the tags typed with it.
TIER="tier 1 — go test"
PROVE="go test ./internal/pdfops/ -count=1 -run TestARoleMapEditSetsReplacesAndRemovesOneMapping"
EXPECT="mapping an unmapped type: the elements read"
