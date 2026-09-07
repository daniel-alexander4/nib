# docs/red-proofs.md, tier 2: "the block outline is positioned against the column" (P02.S03, v1.128.8)
#
# The defect: the outline is appended to `#srvPreview` rather than to its page's wrapper. The
# preview is a scrolling column of pages; an absolutely-positioned box measured against the column
# is correct only while the column is at scroll zero, and slides off its page the moment the user
# scrolls to read the document they are being asked to sign.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/consentblock.test.mjs"
EXPECT="the block was drawn on the wrong page"
