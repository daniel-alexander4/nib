# docs/red-proofs.md, tier 1: "A law figure emitted twice per side" (P07.S09b, D33, v1.117.225)
#
# The defect: each punch path builds its own `&punchBudget{}` again — the state P07.S09b found.
#
# `punchLoop`'s doc says the armed side and the dialing side "share one per-side budget" and
# `punchBudget`'s said a ceremonyID IS a side. Neither was true: the two paths hold different
# ceremonyIDs with different sockets, and P05.S09's symmetric racing has one machine running both
# for one hop. So a side emitted 6,000 packets against D33's 3,000 — a figure D33 calls LAW,
# because under D6's pin an attacker supplies the candidates.
#
# **Nothing observable said so.** Both loops are correct in isolation and every existing punch test
# passes against the defect; the only way to see it is to ask the two paths for their budget and
# compare identity. That is why the guard checks identity first and then exhaustion: the identity
# check names the defect, the exhaustion check proves the identity means what it claims.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestBothPunchLoopsOfOneHopSpendOneBudget"
EXPECT="hold DIFFERENT packet budgets"
