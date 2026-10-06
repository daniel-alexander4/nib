# docs/red-proofs.md, tier 2: "The zoom level shows a background document's zoom (ADR-087, v1.185.0)"
#
# The defect: the readout is shared chrome written from every view's scale event, so a document re-scaling
# in the background overwrites the focused document's zoom on the bar.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/toolbaricons.test.mjs"
EXPECT="now shows ITS zoom"
