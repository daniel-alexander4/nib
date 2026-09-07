# docs/red-proofs.md, tier 2: "the block outline ignores the preview scale" (P02.S03, v1.128.8)
#
# The defect: the outline is drawn at scale 1 instead of the viewport's own
# `vp.width / base.width`. The preview fits pages to 380px, so at US Letter that is 0.62 — the box
# is then 60% too large and in the wrong place, and its SIZE is what the appearance image is
# rasterised to, so it misreports what will be stamped as well as where.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/consentblock.test.mjs"
EXPECT="the block's top is"
