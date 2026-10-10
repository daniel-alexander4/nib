# docs/red-proofs.md, tier 2: "Choosing in a role map row's picker applies the mapping (ADR-127)"
#
# The defect: the picker sends the edit on change, so every arrow press a keyboard user makes through its options rewrites the document.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/tagrolemap.test.mjs"
EXPECT="choosing in a picker sent an edit"
