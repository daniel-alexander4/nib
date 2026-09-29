# docs/red-proofs.md, tier 1: "a signed hybrid-reference file reads as unsigned" (/pending 733, v1.169.5)
#
# The defect: signatureBlobPresent walks /Fields through digitorus/pdf even when the file names /XRefStm.
# That library never follows a hybrid trailer's /XRefStm, so the signature dictionary is invisible to it
# and a walk that found no field was read as a file with no signature.
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run '^TestASignedHybridReferenceFileIsNeverUnsigned$' -count=1"
EXPECT="HasSignatureBlob = false for a signed hybrid-reference file"
