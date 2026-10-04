# docs/red-proofs.md, tier 2: /pending 738 — the consent screen names a refused signature
#
# The defect: the consent view is handed the signers and not the refusals, so the one reader of the
# document's signatures that a party consults before adding their own says nothing about a signature
# Nib refused — the badge, the details panel and `nib verify` all name it.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/consentroster.test.mjs"
EXPECT="the consent screen does not say so (/pending 738)"
