# docs/red-proofs.md, tier 3: "the way back is painted under the document" (P03.S04, v1.128.45)
#
# `.viewerContainer` is positioned with `z-index: auto` and so creates no stacking context, which
# puts every page overlay in `#viewerWrap`'s own context — `.ovl` 8, its stamp/box/shape/note
# variants 9, `.shapemark` 10. At `#signBanner`'s rank of 6 the parked-setup bar loses to all of
# them, and they are pointer-interactive: a stamp near the bottom-left of the visible page covers
# "Back to setup" and takes the click. #signBanner can afford 6 because its button is a
# convenience; this bar carries the only route back to a half-filled ceremony.
#
# **The only genuinely tier-3-only half of this property, and the reason it needed an assertion of
# its own.** jsdom has no layout, so `hidden`, `closest()` and even the element's rect are all
# unchanged by this patch; the sibling row `the-way-back-lives-in-the-sidebar` is caught at tier 2
# precisely because it is structural and this one is not. Measured first as a bare Playwright click
# TIMEOUT — a red with no assertion behind it, which `redproof.sh` correctly refuses to accept as a
# proof — so the test now asks `elementFromPoint` directly and the defect has a sentence.
TIER="tier 3 — the real binary in a real browser"
PROVE="./build/uirepro.sh"
EXPECT="the way back is painted under the document"
