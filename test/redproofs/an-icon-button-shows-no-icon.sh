# docs/red-proofs.md, tier 3: "an icon button shows no icon" (v1.125.2)
#
# `.tbicon`'s svg sizing goes away, which is the state Find and Reload SHIPPED in: the class was
# introduced with the reload button (v1.123.5) and never given a rule, so the `<svg>` inside had no
# width or height and rendered at **0x0** — measured, not estimated. Both buttons were padding
# around nothing and read as blank gaps in the bar.
#
# **It shipped past every tier**, and the reason is the whole point of this row: the element is in
# the DOM, so the structural tier calls the icon present, and jsdom has no layout to measure it
# with. Only a rendered box can tell an icon from an element that exists. It was reported by Dan.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="renders at no usable size"
