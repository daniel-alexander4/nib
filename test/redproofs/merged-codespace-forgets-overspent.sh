# docs/red-proofs.md, tier 1: "an overspent codespace is read once it is merged" (/pending 724, v1.169.10)
#
# The defect: Codespace.Merge does not carry Overspent, so a codespace refused for costing the index more
# than its budget is answered from as though whole once a usecmap chain merges it. The deterministic
# half of the test; its timing ratio runs first and is not what this row asserts.
TIER="tier 1 — go test"
PROVE="go test ./internal/fontcode/ -run '^TestAWideCodespaceIsReadInLinearTime$' -count=1"
EXPECT="an overspent codespace stopped being refused once merged or cloned"
