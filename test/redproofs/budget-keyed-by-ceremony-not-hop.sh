# docs/red-proofs.md, tier 1: "D33's LAW figure keyed by ceremony rather than by hop"
# (P08.S05h, D33, v1.117.323)
#
# The defect: the shipped one, for two phases. `punchBudgetFor(c.inv.ID)` keyed the budget by the
# ceremony, which is the form D33 STRUCK by amendment — *"Total punch budget = 3,000 packets per
# ~~ceremony~~ HOP"* — and the amendment gives the reason: *"a per-ceremony budget was exhausted
# inside the first hop … in a 31-hop ceremony hops 2–31 would get zero packets."*
#
# **Two guards already sat on the neighbouring axes and neither could see this one.** One pins that
# both loops of ONE hop share a budget; the other pins that two CEREMONIES do not. The unit is one
# indirection from each, and the second row's own text says "D33's unit is per HOP" while guarding
# a process-wide counter. `hopScoped(c, cer.hop)` filters the candidate stream by hop eight lines
# from where the budget was taken by ceremony.
#
# **The failure is silent by construction**, which is why nothing observable caught it: dropping
# over the cap is the designed behaviour, so a starved hop punches less and then not at all, and
# reports drops indistinguishable from a hop that legitimately spent its own share.
#
# The mutation multiplies the hop by zero rather than dropping the term, because dropping it leaves
# `strconv` unused and the package fails to COMPILE — a red for the wrong reason, and the first cut
# of this row was exactly that.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTwoHopsOfOneCeremonyDoNotShareABudget -count=1"
EXPECT="hold the SAME packet budget"
