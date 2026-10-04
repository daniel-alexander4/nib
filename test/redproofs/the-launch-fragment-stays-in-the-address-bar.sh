# docs/red-proofs.md, tier 2: "the launch key is traded while it is still in the address bar, so a reload or a bookmark replays a spent key"
# (/pending 685)
#
# Recorded at /pending 637/700 from a probe that was run by hand when the fix landed and never
# written down as a replayable row.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/launchkey.test.mjs"
EXPECT="actual: '#k=the-launch-key'"
