# docs/red-proofs.md, tier 2: "A toolbar button shows its word beside its icon (ADR-087, v1.185.0)"
#
# The defect: a button in the fixed bar carries its label as bare text instead of in the `.tblabel` span,
# so the word is drawn in the bar — the shape the icon bar decays in, one "this one needs a word" at a time.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/toolbaricons.test.mjs"
EXPECT="shows a word in the bar"
