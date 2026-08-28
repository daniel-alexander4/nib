# docs/red-proofs.md, tier 2: "A denominator that counts the panel's own rows" (P07.S07c, v1.117.221)
#
# The defect: "X of Y signature(s) name a ceremony" takes Y from the rows the panel drew rather
# than from the signatures Go reported.
#
# **This row's guard could not fail on its first attempt, and the fix was the FIXTURE.** Driven
# against nine ceremony signatures, the rows drawn and the signatures reported are both nine, so
# the two expressions are the same number and the patch changes nothing. It needs a signature
# carrying no attestation at all — an ordinary Finalize, which any party can add — which no row
# draws and Go still counts. That is the only shape separating the two, and without it the guard
# was a green that could not go red.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="the denominator is not the 10 signatures Go reports"
