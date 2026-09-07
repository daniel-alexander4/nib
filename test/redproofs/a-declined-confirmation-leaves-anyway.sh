# docs/red-proofs.md, tier 2: "a declined confirmation leaves anyway" (P05.S01, v1.128.10)
#
# The defect: the confirmation's answer is ignored. Leaving forgets the invitation and cannot be
# undone without a fresh one, so this is a destructive action taken against the user's stated no.
#
# **The assertion is on the REQUEST and deliberately not on the DOM**, because a harness that
# answers dialogs on the user's behalf makes "the card is still there" true whether or not anything
# was sent — measured on this repo at /pending 333, where a test meant to decline an overwrite was
# accepting it. Counting the calls to the route cannot be satisfied that way.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonyleave.test.mjs"
EXPECT="still left the ceremony"
