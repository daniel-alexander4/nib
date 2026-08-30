# docs/red-proofs.md, tier 1: "the arrival gate reserves a whole hop" (P08.S04a, v1.117.282)
#
# **The DISCRIMINATING proof, and the reason the slice has three arms instead of two.** The gate is
# still present and still refuses expired ceremonies — only its budget changes, from 0 to
# `ceremonyHopBudget()`. So the "expired is refused" arm stays GREEN and only the honest-worst-case
# arm goes red. Nothing else in the tree can tell "budget too large" from "feature absent".
#
# It is not hypothetical: this is the deepdive's own error, made falsifiable. The convener admits a
# hop at 29m20s and the signer's gate runs at worst 22m20s, so a legitimately admitted hop arrives
# with as little as 7m00s left — and any reservation at this end refuses it, with neither party at
# fault. The margin IS the clock-skew tolerance; spending it costs the whole thing.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheArrivalGateRefusesAnEndedProceeding -count=1"
EXPECT="at the convener's worst case was refused"
