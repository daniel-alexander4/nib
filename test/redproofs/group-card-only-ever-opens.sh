# docs/red-proofs.md, tier 2: "a group card only ever opens" (ADR-020, v1.123.1)
#
# The mirror of `panel-card-only-ever-opens`, against the OTHER kind of card: `openCard` loses the
# close branch it has carried since v1.122.0, so *Fill Forms from Data* and every other command
# group re-opens on a second click.
#
# Recorded as its own row because the guard's whole claim is that it reads both kinds. A guard that
# happened to walk only the panel cards would be green under this patch and would have been green
# through the defect ADR-020 fixes; the two rows together are what says it walks the set.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/tablist.test.mjs"
EXPECT="stayed open when their own header was clicked a second time"
