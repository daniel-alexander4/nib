# docs/red-proofs.md, tier 2: "the waiting card loses the cascade" (/pending 377, v1.128.25)
#
# The defect: the degraded-card border rule stops excluding `[data-waiting]`, which is how this fix
# was FIRST written — a later `.cercard[data-waiting] { border-color: … }` meant to override it.
# That override loses: `.cercard[data-state]:not([data-state="ok"])` is one class and TWO
# attributes, because `:not()` carries its argument's specificity, against the override's two. So
# the peach border stays on a card whose badge has just said nothing is wrong.
#
# **Recorded because nothing below tier 3 renders CSS.** `boot.mjs` loads no stylesheet, so
# `getComputedStyle` cannot answer here and the badge/attribute assertions in this file would all
# have passed against the broken version. The guard reads the RULE instead, which is the same thing
# `cardhue.test.mjs` does for its ladder and for the same stated reason.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/prehopcard.test.mjs"
EXPECT="must exclude a waiting card"
