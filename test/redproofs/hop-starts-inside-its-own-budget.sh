# docs/red-proofs.md, tier 1: "The ceremony deadline compared against `now` alone"
# (P05 sweep, v1.117.37 — D16's clock 3)
#
# The defect: `checkCeremonyDeadline` asked whether the ceremony record had EXPIRED, which is
# a comparison against `now`. D16's clock 3 nests inside clock 2: the record must outlive one
# whole exchange budget, not merely be unexpired at the instant the hop starts.
#
# What it costs: a hop begins with three minutes left on a six-minute budget. It is not
# expired — which is why comparing against `now` passes it — and it cannot finish. The far
# party is asked to consent to a signature on a proceeding that ends before the exchange does.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestAHopDoesNotStartAfterTheCeremonyCanOutliveIt"
EXPECT="cannot finish"
