# docs/red-proofs.md, tier 3: "the sidebar cards inherit the toolbar's gap" (v1.123.2)
#
# `#commands .tbtab` goes back to `gap: 10px` — the state the product shipped in from v1.122.0,
# when the accordion was built on panes that ADR-017 moves between the bar and the column. The gap
# separates GROUPS sitting side by side in a horizontal toolbar; stacked as cards it becomes a 10px
# band of `--mantle` between every pill, and between a card's head and its own open body.
#
# It is the FOURTH instance of one class — a rule written for the bar travelling into the column —
# and ADR-018 records the other three. Nothing functional fails under this patch: every card still
# opens, closes and gates by mode. It only looks wrong, which is why it survived four versions.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="do not touch their neighbour"
