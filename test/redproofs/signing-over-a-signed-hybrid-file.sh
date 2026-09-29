# docs/red-proofs.md, tier 1: "signing a signed hybrid-reference file" (/pending 740, v1.169.15)
#
# The defect: readableBySigner rewrites a hybrid-reference file even when it already carries a signature,
# so the rewrite destroys that signature (or the library writes an unreadable file over one it could not see).
TIER="tier 1 — go test"
PROVE="go test ./internal/sign/ -run '^TestASignedHybridReferenceFileIsRefusedByName$' -count=1"
EXPECT="want ErrSignedHybridReference (/pending 740)"
