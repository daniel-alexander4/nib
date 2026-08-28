# docs/red-proofs.md, tier 1: "The per-hop ceiling becomes a lifetime allowance" (P07.S09b, D33, v1.117.225)
#
# The defect: the budget registry keys everything under one entry, so every ceremony on this
# machine shares one 3,000.
#
# The opposite error to the row beside it, and it is the one a careless fix produces: having found
# that two paths must share a budget, the tempting shape is one counter. D33's unit is per HOP, so
# a process-wide counter turns the ceiling into a lifetime allowance — a machine that has run
# ceremonies all day punches not at all for the next one, and the starvation is silent because
# dropping is the designed behaviour.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTwoCeremoniesDoNotShareABudget"
EXPECT="share one packet budget"
