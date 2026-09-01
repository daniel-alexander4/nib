# docs/red-proofs.md, tier 1: "a block line never wraps" (/pending 286, v1.117.307)
#
# `wrapBlockLine` returns the whole string as one entry however wide it is — the pre-286 renderer,
# where `AppearanceLines` emitted one entry per field and nothing wrapped.
#
# What that costs is the defect the item was filed for: `web/app.js` draws each line with
# `ctx.fillText` and **no `maxWidth`**, so the canvas clips at its bounds. A recital of ordinary
# length for a lease is then either refused outright (the old behaviour, which is why four separate
# one-line ceilings existed) or cut mid-word above somebody's signature.
#
# It is the FEATURE half of the item rather than a guard against regression in a bound: with this
# applied, every block is exactly what it was before wrapping shipped.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestABlockLineWraps -count=1"
EXPECT="it was not wrapped"
