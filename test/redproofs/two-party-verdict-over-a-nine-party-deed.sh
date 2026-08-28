# docs/red-proofs.md, tier 2: "A nine-party deed described as a private exchange" (P07.S07c, C09, v1.117.221)
#
# The defect: the mutual-co-sign branch takes the baton condition, so "each party's signature
# attests to the OTHER's key" prints over a nine-party ceremony.
#
# `matched` is per-pair, so on a completed baton every signature after the first matches its
# predecessor and the condition holds. "Mutually" is false twice over — party 1 accepts nobody
# and nobody accepts party 9 — and a reader checking a nine-party deed is told it is an exchange
# between two people.
#
# The companion control matters as much: the sentence is TRUE of a mutual pair, and
# `oneproceeding.test.mjs` drives such a document carrying roster tokens and asserts the positive
# survives. The discriminator is the document's shape, not whether it carries a record.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="described as a mutual exchange between two people"
