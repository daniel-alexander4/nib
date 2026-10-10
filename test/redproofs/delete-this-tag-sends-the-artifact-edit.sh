# docs/red-proofs.md, tier 2: "Delete this tag sends the artifact edit (ADR-124)"
#
# The defect: the button that promises to keep a tag's content sends the one edit that takes the content out of the tree.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/tagedit.test.mjs"
EXPECT="the edit sent is not a delete of the selected element"
