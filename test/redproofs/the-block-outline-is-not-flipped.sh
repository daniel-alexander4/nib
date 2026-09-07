# docs/red-proofs.md, tier 2: "the block outline is not flipped" (P02.S03, v1.128.8)
#
# The defect: the outline's CSS `top` is taken as `lly * scale` instead of
# `vp.height - ury * scale`. A PDF rect measures from the BOTTOM left and a canvas from the top
# left.
#
# **This is the row the tier exists for.** It does not throw, it does not fail a Go test, and the
# box is exactly the right SIZE — it is simply in the wrong half of the page, on the screen where a
# signer is being told where their signature will go. The Go rows above prove the numbers are the
# ones that will be stamped; nothing there can see what those numbers become on screen.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/consentblock.test.mjs"
EXPECT="which is the upper half"
