# docs/red-proofs.md, tier 2: "leaving is offered after signing" (P05.S01, v1.128.10)
#
# The defect: the control's population loses its `state !== 'ok'` clause, so it appears on
# ceremonies this machine has signed — where the server refuses it. An offer the app knows it
# cannot keep, and the user learns that only by pressing it.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonyleave.test.mjs"
EXPECT="the control is offered on a ceremony this machine has signed"
