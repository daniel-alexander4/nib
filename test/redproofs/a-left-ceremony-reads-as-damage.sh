# docs/red-proofs.md, tier 2: "a left ceremony reads as damage" (P05.S01, v1.128.10)
#
# The defect: the receipt renderer has no word for `left`, so it falls to the chain's last arm —
# "Ended in a way this version does not recognise". That is a sentence about a damaged file, shown
# to a user for the thing they did on purpose a moment earlier.
#
# **This was a real defect in this slice, not a manufactured mutation.** It shipped in the first
# cut and was found by writing this case. Adding the word then staled
# `abandoned-borrows-the-unknown-sentence`, which patches the same ternary chain — caught in the
# same run by the root package's TestEveryRedProofStillApplies, and re-recorded.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonyleave.test.mjs"
EXPECT="does not say so"
