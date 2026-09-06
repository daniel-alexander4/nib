# docs/red-proofs.md, tier 2: "a mode entry bypasses the show door" (ADR-020, v1.123.1)
#
# `syncSidebarForMode` goes around `showPanel()` and clicks the header itself — the shape the code
# had before ADR-020, when a header click was not yet a toggle and clicking one was harmless.
#
# It is the ADR-009 half: the rule is the guard, not the caller, so this row asks whether a SECOND
# site can reintroduce the defect the first site was fixed for. Same outcome as
# `showpanel-toggles-instead-of-showing`, reached through a different door — which is the point.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/tablist.test.mjs"
EXPECT="entering a mode that is already showing its landing panel CLOSED it"
