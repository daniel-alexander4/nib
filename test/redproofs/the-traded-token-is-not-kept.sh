# docs/red-proofs.md, tier 2: "the traded token is not kept where a reload of the tab finds it"
# (/pending 685)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/launchkey.test.mjs"
EXPECT="not ok 3 - the token is kept where a reload of this tab finds it"
