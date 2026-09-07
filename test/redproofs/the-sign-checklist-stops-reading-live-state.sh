# docs/red-proofs.md, tier 3: "the sign checklist stops reading live state" (ADR-027, v1.127.0)
#
# `renderSignSteps` is dropped from the funnel that fires on every document load, client edit and
# save, so the checklist paints once and never again.
#
# **It still LOOKS right**, which is the whole reason for the row: the rows are there, the required
# badges are there, the ticks it drew at boot are correct for boot. It goes wrong only as you work
# — you open a document and the list still says you have not. A checklist that does not track is
# decoration, and decoration that looks like state is worse than nothing.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="still shows that step as outstanding"
