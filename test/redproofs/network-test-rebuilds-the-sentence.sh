# docs/red-proofs.md, tier 2: "the network test speaks in the server's words" (/pending 23, v1.117.352)
#
# The defect: the client rebuilds a sentence from the verdict tag instead of rendering `summary`.
#
# The rule that decides which of the four states obtains has ONE door — `discovery.Stats.Verdict()`
# — and the wording travels with it, so `nib discover` and the panel cannot drift. A client that
# writes its own prose is a second copy of that rule, and it drifts the day either is edited.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/networktest.test.mjs"
EXPECT="server's own sentence is not rendered"
