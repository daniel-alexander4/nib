# docs/red-proofs.md, tier 2: "a refused edit names the ceremony" (P06.S09, D29, v1.117.348)
#
# The defect: `toast('page operation failed')`. The server-side freeze was built at P07.S02a and is
# guarded for ROUTING — every mutating route reaches it — but nothing asked whether the user ever
# reads the refusal, and they did not. D29's rule is that the refusal NAMES the proceeding; at this
# route it named nothing, so a user whose every edit was refused had no account of why.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/freezesurface.test.mjs"
EXPECT="a refused edit said"
