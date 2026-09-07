# docs/red-proofs.md, tier 3: "the title stops following the save flag" (ADR-024, v1.125.0)
#
# `setDirty` stops repainting, so the toolbar's save dot is whatever it was at the last paint.
#
# **The failure it reproduces is the one this door was built for**, and it is not "the dot goes
# stale after an edit" — it is that a FRESHLY OPENED document reads "Unsaved changes". Every
# arrival goes through `setDocumentFromServer`, which marks the view dirty; `installOpened`
# corrects it a line later. Painted from the assignment rather than through one door, the bar
# says the opposite of what the close prompt would.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="reads as having unsaved changes"
