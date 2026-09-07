# docs/red-proofs.md, tier 2: "the lifecycle creeps back into the bar" (ADR-022, v1.124.0)
#
# Close Document is moved out of File mode and back into the fixed toolbar — the shape this decays
# in. It is a one-line-looking diff, it is locally reasonable ("Close is important"), and it costs
# a toolbar row at every width; ADR-017's own measurement is that putting a pane back in the bar
# reached 34.8% of the viewport at 800px against a 33% ceiling.
#
# The guard is structural and reads the DOM rather than a list of labels, because the control that
# comes back will be whichever one somebody argues for, not one a name list predicted.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/toolbargroups.test.mjs"
EXPECT="are back in the fixed bar"
