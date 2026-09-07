# docs/red-proofs.md, tier 2: "the Pages tab opens onto nothing" (ADR-024, v1.125.0)
#
# `buildSidebarTabs` sends every panel to Functions, so the Pages section is an empty container
# behind a tab that still looks clickable.
#
# The guard is structural and reads the DOM: the split is made by MOVING nodes at boot, so the way
# it decays is a panel landing in the wrong section — which renders as a tab that opens onto blank
# column and nothing else wrong anywhere.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/toolbargroups.test.mjs"
EXPECT="not inside the Pages section"
