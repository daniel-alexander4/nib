# docs/red-proofs.md, tier 2: "a retired flavour reaches the DOM" (ADR-021, v1.123.4)
#
# `applyAppearance` stops normalising, so a vault holding `frappe` or `macchiato` — real strings
# in real vaults, written while Nib offered four flavours — puts that value on <html> after the
# palette for it has been deleted.
#
# **It looks correct, which is the whole reason for the row.** With no matching
# `:root[data-appearance="frappe"]` block the stylesheet falls through to the bare `:root` tokens
# and renders Mocha: the right pixels, by luck, under an attribute naming a theme nothing defines.
# The next rule keyed on that attribute inherits a bug with no symptom today.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/themefallback.test.mjs"
EXPECT="left data-appearance="
