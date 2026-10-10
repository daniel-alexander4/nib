# docs/red-proofs.md, tier 2: "Make inline tags editable sends an edit naming the selected tag (ADR-126)"
#
# The defect: the button sends the selected tag's id — 0 — with the promote edit, which the server refuses: the one way out of the dead end is itself a dead end.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/tagpromote.test.mjs"
EXPECT="the button did not send exactly one promote edit that names no element"
