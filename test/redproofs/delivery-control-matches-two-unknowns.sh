# docs/red-proofs.md, tier 2: "the delivery control never matches two unknowns" (/pending 353, v1.117.351)
#
# The defect: the gate compares `(c.me || '')` to `(c.convener || '')`, so a ceremony that knows
# NEITHER its position nor its convener matches on '' === '' and is offered the round.
#
# **This row exists because the first version of its test could not catch it.** The fixture only
# dropped `me`, leaving `convener` known — so the comparison was false for the wrong reason and the
# mutation survived. The surviving mutation was the proof of a coverage hole, and the fifth
# ceremony in that file is the fix. Both fields are unknown-when-empty by their own doctrine.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonydeliver.test.mjs"
EXPECT="knows NEITHER its position nor its convener"
