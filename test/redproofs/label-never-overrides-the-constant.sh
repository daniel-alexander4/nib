# docs/red-proofs.md, tier 1: "Nine blocks that all say the same name" (P07.S07a, v1.117.216)
#
# The defect: `StampCommitment` stops overriding `att.Signer`, so a block reports the signing
# CERTIFICATE's common name. In the product that constant is `"Nib User"` at every machine, so a
# nine-party ceremony renders nine identical blocks — and `Party.Label`, which the convener typed
# and every party's signature commits to, is read by nothing.
#
# **Every assertion about ONE block passes against this.** The block exists, it is on the page its
# roster position allocates, its rect matches, it carries an appearance stream, and the name in it
# is a well-formed signer name. What fails is the comparison BETWEEN blocks, which is why the
# guard counts distinct blocks rather than checking one.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestNineBlocksNameNinePartiesAndNotOneOfThemIsNibUser"
EXPECT="the label is inside the signed commitment and the block is reading something else"
