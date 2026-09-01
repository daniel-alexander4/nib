# docs/red-proofs.md, tier 1: "the contribution is stored after the frame goes out" (/pending 343, v1.117.300)
#
# **The pre-P08.S02 ordering, restored.** `rd.Store` moves out of `coSignExchange` and into `Receive`
# AFTER `writeFrame` — which is exactly where the write lived before the slice, as `mirrorHop` from
# `openArrival`, best-effort and second. The bytes reach the peer first and the disk second, so a
# crash in between leaves a signature the counterparty holds and this machine has no record of.
#
# Every other check in the tree stays green: the document is identical, both signatures verify, the
# cache is populated, and `l3_test.go`'s scan still finds `persistContribution(` inside `Store` —
# because `Store` is unchanged. Only the ORDER moved, and only a reader on the far side of the wire
# can see an order.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestTheContributionIsStoredBeforeTheFrameGoesOut -count=1"
EXPECT="durable write is still in flight"
