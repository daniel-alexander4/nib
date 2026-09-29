# docs/red-proofs.md, tier 1: "a signature nib could not check says nothing" (/pending 741, v1.169.17)
#
# The defect: Verify never sets Status.Unchecked, so a document that is Invalid with no signer, refusal or
# timestamp to name gives the user a verdict and no reason.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run '^TestAnUncheckedSignatureSaysWhy$' -count=1"
EXPECT="(/pending 741)"
