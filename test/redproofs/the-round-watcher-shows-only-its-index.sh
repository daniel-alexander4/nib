# docs/red-proofs.md, tier 2: "a stalled leg still reports progress" (/pending 370, v1.117.354)
#
# The defect: the watcher renders "Reaching Bob — 2 of 4" and drops the elapsed time and its ceiling.
#
# **The index is the number that does NOT move.** A party who is not listening holds one leg for the
# whole connect deadline — 300 s, measured at tier 4 — so a surface reporting only the position is
# silent for exactly the minutes this item is about, which is when the convener starts wondering
# whether the round has hung. Elapsed against a stated bound is the pair that answers it: without the
# bound a rising number is not progress, it is just a number going up.
TIER="tier 2 — jsdom"
PROVE="node --test test/jsdom/ceremonydeliver.test.mjs"
EXPECT="elapsed time and its ceiling are not shown together"
