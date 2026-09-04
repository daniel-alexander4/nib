# docs/red-proofs.md, tier 2: "nobody else here is not success" (/pending 23, v1.117.352)
#
# The defect: only the two hard failures are styled as warnings, so "the network works and no other
# Nib is announcing" renders as a clean result.
#
# It is an ordinary answer and it is still the reason nothing is happening — the user pressed this
# button because their ceremony was not starting. Only `working` is an answer that needs no action.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/networktest.test.mjs"
EXPECT="styled as success"
