# docs/red-proofs.md, tier 3: "The page-compare hash was reading 72 pixels, not 72 cell means"
# (/pending 276, v1.117.139)
#
# The defect: reduce the rendered page with `ctx.drawImage(canvas, 0, 0, 9, 8)`. The comment
# above that line called it a box filter and it is not one — Chromium's default smoothing on a
# ~150px-to-9px reduction effectively POINT-SAMPLES. A probe printing the grid showed a page of
# nine flat columns coming back as the exact column greys with no blending at all, and a page of
# text coming back as 63 cells of untouched paper and 9 cells of untouched ink. Sensor noise,
# paper texture, halftoning and a page of text averaging to grey are the entire reason a
# perceptual hash exists, and none of them reached the algorithm.
#
# What it costs: a full page of text lands 4 bits from a BLANK sheet against a threshold of 12,
# so a dropped or blank-fed page aligns silently against real content.
#
# **This one hid behind agreement.** The tier-3 instrument reduced the same way, with the same
# line, so the test confirmed the product's mistake back to it and four green tiers saw nothing.
# The test now calls the product's own `gridMeans`, and `gridMeans` averages every source pixel.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="a dropped or blank-fed sheet would align against a page of content"
