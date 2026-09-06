# docs/red-proofs.md, tier 2: "showPanel toggles instead of showing" (ADR-020, v1.123.1)
#
# `showPanel` loses its is-it-already-open guard and becomes a bare `.click()`. With the header now
# a real toggle, "show me this panel" then CLOSES the panel whenever it is already open — so
# re-entering a mode empties the surface it exists to land on.
#
# **This row exists because the mutation survived.** The toggle guard was written first, and
# deleting this guard left all 188 tier-2 tests green: the door's own property had no coverage, and
# the test above it was added in the same change to close the hole its survival proved.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/tablist.test.mjs"
EXPECT="entering a mode that is already showing its landing panel CLOSED it"
