# docs/red-proofs.md, tier 2: "Set spans sends the column span and drops the row span (ADR-119, v1.202.0)"
#
# The defect: the Set spans button sends one edit where the bar shows two fields, so a row span typed there is never sent.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/tagedit.test.mjs"
EXPECT="the edits sent are not the ones the controls describe"
