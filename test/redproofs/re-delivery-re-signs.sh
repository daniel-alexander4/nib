# docs/red-proofs.md, tier 1: "Re-delivery re-signs instead of returning the cached signature" (P05.S10, v1.117.88)
#
# The defect: coSignExchange ignores the ReDeliverer and runs the full Confirm+Contribute every time.
# Because Contribute is non-deterministic (random ECDSA nonce + a wall-clock timestamp), a reconnect
# after a lost writeback then produces a SECOND, DIFFERENT co-signature block over the same document
# — D25's two-blocks-in-one-doc wrong-ness, the exact thing D24's "re-deliver, do not re-sign" and
# S10's idempotent cache exist to prevent. The patch drops the cache lookup so the second co-sign of
# one document re-prompts consent and re-signs.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestReDeliveryIsIdempotent -count=1"
EXPECT="asked again on a re-delivery"
