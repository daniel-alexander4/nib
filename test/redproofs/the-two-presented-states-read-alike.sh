# docs/red-proofs.md, tier 2: "the two presented states read alike" (P02.S04, v1.128.12)
#
# The defect: "the words reached nobody" and "the words were shown and not confirmed" render the
# same sentence. They are different facts — one is a machine that showed nothing, the other is a
# person who looked and did not confirm — and D5's acceptance clause asks for three states, not two.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/spokencheck.test.mjs"
EXPECT="both read as"
