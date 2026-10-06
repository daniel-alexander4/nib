# docs/red-proofs.md, tier 2: "A reload brings back the document the launch opened (ADR-086, v1.184.0)"
#
# The defect: `?open=` stays in the window's address after it is used, so every reload opens that file
# again — including after the user closed it.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/handoffpush.test.mjs"
EXPECT="still in the address"
