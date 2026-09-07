# docs/red-proofs.md, tier 2: "there is no way to leave a ceremony" (P05.S01, v1.128.10)
#
# The defect: the control is never offered. This is the state the slice was built to end — since
# D14 the only lever a party had was quitting Nib, and quitting is a pause rather than a decision
# because the next unlock arms again.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonyleave.test.mjs"
EXPECT="offers no way to leave"
