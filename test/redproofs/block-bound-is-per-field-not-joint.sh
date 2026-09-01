# docs/red-proofs.md, tier 1: "the block bound goes back to per-field" (/pending 286, v1.117.307)
#
# **The DISCRIMINATING row of the four.** `checkBlocksFit` measures each party's block WITHOUT the
# recital, so every field is effectively bounded on its own again. The bound still exists, still
# refuses an over-long capacity, and still names the party — so the two tests about a single
# over-long field stay GREEN.
#
# What fails is the only assertion that can see it: a capacity and a recital that each fit ALONE
# combining into a block that does not. That property could not even be stated before wrapping —
# each field had its own one-line ceiling and nothing asked about their sum — and it is the whole
# reason the bound became joint. A per-field ceiling admits both values and the block it produces
# renders below the legibility floor.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestTheBlockBudgetIsJOINT -count=1"
EXPECT="each fit alone convened together into a block that does not"
