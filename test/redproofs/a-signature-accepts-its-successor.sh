# docs/red-proofs.md, tier 1: "a signature accepts its successor" (P07.S05, v1.117.172)
#
# The defect: `PredecessorOf` points FORWARD — which is how this task was written in the plan
# before the suite refuted it.
#
# `crossBind` sets `Matched` only when the accepted party is itself a valid signer **on this
# document**, and only a predecessor can be: a successor has not signed yet. Accepting forward
# therefore leaves every signature unmatched until the one after it lands, and the last one
# unmatched forever. Measured: three two-party ceremony tests failed with *"peer's signature does
# not accept you"*.
#
# C14 as amended says it in as many words — *"every signature that has a signing predecessor
# reports Matched; the first signer reports its own state"*.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestASignatureAcceptsItsPREDECESSOR -count=1"
EXPECT="want A"
