# docs/red-proofs.md, tier 3: "a tall card clips its own items" (v1.125.4)
#
# The open card's body goes back to `max-height: 60vh; overflow-y: auto` — its own scroller inside
# the sidebar's scroller.
#
# **It does not scroll; it CLIPS, and reports no overflow while doing it.** Measured at a 420px
# window: `clientHeight` 223 against 505px of items, `scrollHeight` also 223. A flex column with a
# capped height clips its children without establishing scroll extent, so the items past the cap
# are unreachable and every DOM property says the box is full rather than overflowing.
#
# Two cheaper observables were tried against this and BOTH passed it: `scrollHeight` (lies, above)
# and the last item's own rect (a clipped element still has a position, and scrolling the pane
# moves that position into view while the item stays hidden behind the clip). The row's check
# hit-tests the item's centre, which is what accounts for the clip.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="cannot be brought into view"
