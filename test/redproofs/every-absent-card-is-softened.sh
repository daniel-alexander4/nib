# docs/red-proofs.md, tier 2: "every absent card is softened" (/pending 377, v1.128.25)
#
# The defect: `cerWaiting` drops the `joined` half and softens every `absent` card. The blanket
# rewording the fix must not become — a folder the user really did delete then reads as a ceremony
# they are taking part in, and the badge and border written for that case stop firing where they
# are the only warning there is.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/prehopcard.test.mjs"
EXPECT="is marked as waiting"
