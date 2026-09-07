# docs/red-proofs.md, tier 3: "the undo button cannot see drawings" (ADR-023, v1.124.1)
#
# `reflectUndoControls` counts only nib's overlay stack again, so the Undo button reads as
# disabled while annotation-editor changes are on the page.
#
# It is a SEPARATE row from the routing because it is a separate failure: the routing decides
# whether the keystroke reaches the change, and this decides whether anything on screen admits
# the change is undoable. Both were wrong, and the button being wrong is what made the routing
# defect look like "there is no history" rather than "the history is not being walked".
TIER="tier 3 — real browser"
PROVE="./build/uirepro.sh"
EXPECT="Undo button is disabled with a drawing on screen"
