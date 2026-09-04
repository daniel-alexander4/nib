# docs/red-proofs.md, tier 2: "the panel renders the ceremony record's label" (P06.S09, D29,
# v1.117.348)
#
# The client half of the label. The server marks the record and the panel has to say so — and the
# test asserts the user's OWN attachment is NOT labelled, because a label on everything
# distinguishes nothing.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/freezesurface.test.mjs"
EXPECT="listed as an anonymous embedded file"
