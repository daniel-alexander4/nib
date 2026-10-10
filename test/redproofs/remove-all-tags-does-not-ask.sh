# docs/red-proofs.md, tier 2: "Remove all tags goes ahead when the question is declined (ADR-120, v1.203.0)"
#
# The defect: the button asks and then removes the tags whatever the answer.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/tagedit.test.mjs"
EXPECT="a declined removal was sent"
