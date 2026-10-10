# docs/red-proofs.md, tier 2: "Tag selected sends every listed piece, not the ticked ones (ADR-125)"
#
# The defect: the region names every piece of the page, so one tick tags the whole page as one element.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/taguntagged.test.mjs"
EXPECT="the edit sent is not one region of the two ticked pieces"
