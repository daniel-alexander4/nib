# docs/red-proofs.md, tier 1: "valid (5 signer(s)), and nothing else" (P07.S10, v1.117.231)
#
# The defect: `nib verify` stops printing the ceremony lines — the state that shipped.
#
# The plan's own sentence for why this matters: "the CLI is the surface a dispute actually uses."
# A stranger handed a nine-party deed and told to check it with Nib got `valid (5 signer(s))`,
# which is TRUE and says nothing about the four obliged parties who never signed. Every signature
# on the document is genuine; the document is still not what it looks like.
TIER="tier 1 — go test"
PROVE="go test ./internal/cli/ -run TestVerifyNamesWhoHasNotSignedAndExitsNonZero"
EXPECT="does not say how many of the roster have signed"
