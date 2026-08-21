# docs/red-proofs.md, tier 1: "The per-source cap removed" (P05.S03's unmet acceptance)
#
# The defect: the race checks only the GLOBAL candidate cap, so the bound is first-come.
# First-come is won by whoever emits fastest — which is the capture attack maxLANCandidates
# closed at the browse level, re-opened one layer up, and under D6 an attacker supplies one
# of the sources. One flooding tier spends the whole budget and the honest tier is never
# dialled.
#
# Invisible for as long as there was one source, because with one source a global cap and a
# per-source cap are the same number.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestOneSourceCannotSpendTheWholeRaceBudget"
EXPECT="it must dial at most"
