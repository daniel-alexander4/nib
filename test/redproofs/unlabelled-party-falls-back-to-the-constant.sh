# docs/red-proofs.md, tier 1: "The fallback invents a person" (P07.S07a, v1.117.216)
#
# The defect: an unlabelled party keeps whatever `att.Signer` arrived with instead of falling back
# to their fingerprint — which on both contribution paths is the `"Nib User"` constant.
#
# The fallback is the fingerprint and not a constant on purpose: an unlabelled party is one the
# convener did not name, and the honest block says which KEY signed rather than inventing a
# person. A ceremony whose convener labelled nobody is otherwise nine identical blocks again,
# through the branch the labelled-roster fixture never takes.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAnUnlabelledPartyFallsBackToItsFingerprintAndNeverToAConstant"
EXPECT="the fallback is the constant"
