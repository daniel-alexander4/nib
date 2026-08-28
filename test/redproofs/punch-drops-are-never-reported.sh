# docs/red-proofs.md, tier 1: "Drop-and-report, without the report" (P07.S09b, D33, v1.117.225)
#
# The defect: the diagnosis stops carrying the punch budget's report — the state that shipped.
#
# `punchBudget.report()`'s own doc said it existed "for D19/S11 to surface"; S11 shipped without
# wiring it, so its only callers were tests. The drop half of D33's drop-and-report worked
# perfectly and the report half did not exist: a hop that hit the packet ceiling trimmed its last
# candidates' retries and told the user only that nothing answered.
#
# **The guard also fixed where the reader lives.** The report was first appended after
# `classifyD19`, reachable only on a ceremony holding a live rendezvous and gate — so the one
# assertion that could show it had a reader needed a live DHT. It is gathered above that scope
# check now, since whether D19 can classify a failure and whether this machine exceeded a law
# figure are unrelated facts.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAnExhaustedBudgetReachesTheDiagnosis"
EXPECT="reports nothing to the user"
