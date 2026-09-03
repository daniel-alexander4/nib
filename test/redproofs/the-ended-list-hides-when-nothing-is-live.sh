# docs/red-proofs.md, tier 2: "a machine whose only ceremonies are finished can still see them"
# (P06.S08, ADR-012, v1.117.347)
#
# The defect: `renderCeremonyPanel` returned early on an empty live list, so the ended list never
# rendered. A machine whose ONLY ceremonies were finished showed "No signing ceremonies on this
# machine yet." and nothing else. ADR-012 MOVES a closed-out ceremony rather than deleting it
# precisely so the party's own signed contribution stays findable — and the panel hid it in exactly
# the case it was written for.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/outcomes.test.mjs"
EXPECT="no ended row rendered"
