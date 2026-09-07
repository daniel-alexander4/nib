# docs/red-proofs.md, tier 2: "the pill stops naming the installed version" (v1.125.1)
#
# The version pill goes back to relabelling itself `Update to v<latest> ↓` when a newer release
# exists — the state the product shipped in until 2026-09-06.
#
# **The defect is that it answers the wrong question at the worst moment.** The pill's standing job
# is to say which Nib this is; it stopped doing that exactly when a second version number entered
# the conversation, so a user looking at a red pill could not tell what they were running without
# opening About. Colour already says an update exists and the tooltip can say which one.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/updatepill.test.mjs"
EXPECT="It must still name the version you are RUNNING"
