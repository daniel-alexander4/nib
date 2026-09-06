# docs/red-proofs.md, tier 3: "the reload icon does not ask" (v1.123.5)
#
# `reloadDiscarding` stops confirming, so the toolbar's ↻ throws away every unsaved change on the
# first click. The button sits one place along from Undo, which is the argument for the confirm:
# the two are neighbours because they are the same idea at different scales — Undo steps back one
# operation, this abandons all of them — and neighbours get clicked by accident.
#
# The banner's own Reload button goes through the same door, so this patch removes the confirm
# from BOTH callers with one edit. That is the door working, not a second defect.
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="threw away unsaved work without asking"
