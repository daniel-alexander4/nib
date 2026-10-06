# docs/red-proofs.md, tier 2: "The Undo button is off although the server has history (ADR-087, v1.185.0)"
#
# The defect: the Undo button reads the client stacks only, so after a page operation — which lives in the
# server's ring — the button is greyed out and the mouse has no way to undo.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/toolbaricons.test.mjs"
EXPECT="the mouse has no way to undo"
