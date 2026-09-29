# docs/red-proofs.md, tier 1: "a signature only the hybrid stream lists is not named" (/pending 749)
#
# The defect: Verify never asks whether pdfcpu's reading of the file holds a signature the sweep made
# no record of, so a file whose classic table lists one signature and whose /XRefStm lists another
# reads Valid over the one the library reached — the other is neither counted nor named.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run '^TestASignatureOnlyTheHybridStreamListsIsNamedBesideTheOnesNibChecked$' -count=1"
EXPECT="(/pending 749)"
