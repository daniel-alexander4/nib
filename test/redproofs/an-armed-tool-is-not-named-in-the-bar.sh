# docs/red-proofs.md, tier 2: "An armed tool is not named in the bar (ADR-087, v1.185.0)"
#
# The defect: the bar's contextual group is never shown, so with the sidebar shut nothing on screen says a
# tool owns the pointer, or which.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/toolbaricons.test.mjs"
EXPECT="the bar says nothing"
