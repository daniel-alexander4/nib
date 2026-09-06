# docs/red-proofs.md, tier 2: "a panel card only ever opens" (ADR-020, v1.123.1)
#
# The sidebar's PANEL cards lose their close branch — the state the product shipped in from
# v1.122.0, when the accordion was built, until 2026-09-06. The tab wiring only ever ADDED
# `.active`, so a second click on *Arrange Pages* or *Jump to Section* re-opened a card that was
# already open and there was no way to put it away. Group cards toggled correctly the whole time,
# which is why it read as "some of the pills work".
#
# Nothing else fails under this patch. Opening still works, the mode still lands on its panel, and
# the accordion still shows one card at a time — the sidebar simply cannot be emptied.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/tablist.test.mjs"
EXPECT="stayed open when their own header was clicked a second time"
