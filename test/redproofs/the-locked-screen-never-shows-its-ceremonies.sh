# docs/red-proofs.md, tier 2: "the ceremony panel renders with the vault locked" (P06.S07, D29,
# v1.117.346)
#
# The defect: the SHIPPED one, and P06.S02 marked this criterion met over it. Nothing had rendered
# the PANEL locked: the jsdom boot defaults to `state: 'ready'`, and the one file that overrides it
# asserts the overlay's own content and nothing behind it — so a criterion about a locked surface
# was credited by tests that never reached it in that state.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/lockedpanel.test.mjs"
EXPECT="the ceremonies box is hidden on a locked machine"
