# docs/red-proofs.md, tier 2: "the diagnosis outlives its reason" (/pending 349, v1.117.309)
#
# `reflectDiagnosis` stops CLEARING on absence, taking `reflectNotice`'s rule by analogy.
#
# **The analogy is the defect.** A notice is a sticky record of something that already failed, and
# clearing it on a poll that happens not to carry one would erase a failure the user has not read —
# so that reader deliberately never clears. A diagnosis is LIVE STATE, the answer to "why is nothing
# happening *now*", and when the server stops sending one the reason has stopped applying. Left up,
# it explains a condition that has passed to a user who is still waiting.
#
# Opposite fields, opposite rules. This row is why the two readers are separate functions rather
# than one with a flag.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="a stale explanation of a condition that has passed"
