# docs/red-proofs.md, tier 2: "a name replaces the recital" (v1.128.107)
#
# The defect: the card is headed by this machine's name for the ceremony and the Intent is not
# rendered at all. D20 makes the Intent the recital's only home — it is the sentence every party
# signs — so a panel that shows only a label somebody typed locally is the product paraphrasing
# what was agreed, in the one place a user goes to read it.
#
# **It fails in the invisible direction, which is why it is recorded.** A card headed by a good
# name looks entirely correct; nothing is missing that a reader would notice, and the machine that
# has the name is the machine whose user wrote it. The two strings have to be asserted together,
# on one card, or this passes.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonyname.test.mjs"
EXPECT="HID the recital"
