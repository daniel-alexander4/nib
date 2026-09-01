# docs/red-proofs.md, tier 1: "an unbreakable token wastes a block line" (/pending 286, v1.117.307)
#
# The half-line floor is removed from `longestPrefixThatFits`, so the wrapper takes ANY word
# boundary it can find. On a long unbroken token the only space is the one after the prefix, so
# `Signer: ZZZZ…` breaks after `Signer:` and spends an entire line rendering seven characters.
#
# **It is a real bug that this test found during the build, not a hypothetical.** On a block whose
# HEIGHT is the legibility budget, a wasted line is a smaller font for every other line — so a
# cosmetic-looking wrap choice spends part of the thing `maxBlockLines` exists to protect.
#
# The text still round-trips and every line still fits, so an assertion about correctness alone
# passes here. The check is about the wrapper making progress worth the line it costs.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAnUnbreakableTokenIsHardBroken -count=1"
EXPECT="spending a line to render almost nothing"
