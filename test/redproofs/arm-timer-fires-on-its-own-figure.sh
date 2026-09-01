# docs/red-proofs.md, tier 1: "the arm timer fires on a figure of its own" (/pending 344, v1.117.301)
#
# **The DISCRIMINATING row of the pair.** `runCeremonyReceive`'s deadline stops coming from
# `armWindowFor` while both arm doors still stamp `until` from it — so the STATUS reports a 30-day
# window, the panel counts down from 30 days, and the loop gives up after five minutes. Every
# behavioural assertion about the reported figure stays GREEN, because the reported figure is right;
# what is wrong is that nothing fires on it.
#
# `armWindowFor`'s own comment states this as the property — "both `runSession` and
# `runCeremonyReceive` compute their own timer from this … so the figure the status reports and the
# figure the timer fires on cannot drift" — and until /pending 344 nothing asserted it: six
# production sites, zero test sites.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestEveryArmTakesItsWindowFromOneDoor -count=1"
EXPECT="computes its own arm deadline instead of taking it from armWindowFor"
