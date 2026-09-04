# docs/red-proofs.md, tier 2: "each of D28's end states says its own thing" (P06.S08, v1.117.347)
#
# The defect: the shipped fallback. `renderEndedCeremonies` had no `abandoned` arm, so the sentence
# describing a proceeding that ended in silence was ALSO the sentence for a receipt this build does
# not recognise. Those are different facts and only one of them is about the ceremony — and the
# criterion says each end state produces its OWN message.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/outcomes.test.mjs"
EXPECT="both render as"
