# docs/red-proofs.md, tier 2: "Eight rows for nine signatures" (P07.S07c, C09/C14, v1.117.221)
#
# The defect: `augmentSigDetails` returns early on `!a.acceptedPeer`, which is exactly the FIRST
# SIGNER of a ceremony — `PredecessorOf` returns "" for them because there is nobody before them.
#
# Measured under the patch: nine signatures, eight rows, and the chain sentence then says "8
# parties". The dropped party leaves no trace in the text, so every assertion about the CONTENT
# of a row passes; only counting the rows finds it.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="attestation row(s) for 9 signatures"
