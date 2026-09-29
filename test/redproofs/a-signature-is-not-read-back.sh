# docs/red-proofs.md, tier 1: "a signature nib cannot read back is returned as a success" (/pending 747, v1.169.17)
#
# The defect: runSign ignores signedAsIntended, so a document the library signed without error but that
# nib's own revision sweep reads as broken is handed to the user as signed.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run '^TestASignatureNibCannotReadBackIsRefused$' -count=1"
EXPECT="want ErrSignedOutputUnreadable and nothing (/pending 747)"
