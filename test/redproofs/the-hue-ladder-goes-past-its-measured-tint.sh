# docs/red-proofs.md, tier 2: "the hue ladder goes past its measured tint" (ADR-025, v1.126.0)
#
# The darkest rung of the single-hue ladder is pushed to 1.6x `--card-tint`.
#
# **The whole safety argument for the ladder is that it is BOUNDED by a number already measured.**
# ADR-019 computed the card tint per theme (Mocha 28%, Latte 22%) and found Latte's red passing by
# 0.02 at 26% — no margin at all. Every rung is a fraction of that level, so the darkest is exactly
# today's card and the rest are lighter, which can only raise contrast. A rung above 1.0 leaves the
# range anything was computed at, and there is nothing behind it.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/theme.test.mjs"
EXPECT="past the level every contrast figure in this file was computed at"
