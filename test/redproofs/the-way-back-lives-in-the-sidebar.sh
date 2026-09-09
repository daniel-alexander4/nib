# docs/red-proofs.md, tier 2: "the way back lives in the sidebar" (P03.S04, v1.128.45)
#
# While setup is parked the user is on the document with a half-filled ceremony in memory, and the
# bar offering the way back is the only thing on screen that says so. Putting it in a sidebar panel
# is the obvious placement and it is wrong: `#sidebar.collapsed { display: none }` (style.css) and a
# crossing listener collapses the sidebar automatically below 899px (app.js), so narrowing the
# window mid-excursion takes the only route back with it.
#
# **Recorded as tier 2 after being RUN, having first been written as a tier-3 row on the reasoning
# that the defect needs computed style.** It does not: moving the bar out of `#viewerWrap` is a
# STRUCTURAL change and `bar().closest('#viewerWrap')` sees it for free, one tier down and in a
# second. The tier-3 test does go red for this patch too — but on its geometry assertion, several
# lines before the sidebar is ever collapsed, so the token this row would have declared never
# printed. The tier-3-only half of the property has its own row:
# `the-way-back-is-painted-under-the-document`.
TIER="tier 2 — jsdom"
PROVE="./build/jsdomtest.sh"
EXPECT="the way back is not inside #viewerWrap"
