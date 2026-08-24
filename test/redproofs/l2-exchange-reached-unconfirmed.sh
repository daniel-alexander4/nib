# docs/red-proofs.md, tier 1: "A path reaches the exchange unconfirmed" (L2, /pending 278)
#
# The defect: Initiate stops calling runVerification, so the document exchange is reached
# without the spoken four-word check ever being put to the user. L2 is "no silent
# downgrade — no path completes a ceremony at less than the stated guarantee".
#
# **The assertion that fires is the second one, not the first.** The guard drives four
# entry points with a DECLINING verifier and asserts both that the call fails with
# ErrVerificationDeclined and that the verifier was actually CALLED. A path that never
# asks would also fail — for some other reason — and would satisfy the first assertion
# while being the exact defect L2 forbids. This row is what proves the second assertion
# is the load-bearing one.
#
# Fires on both transports, because the ordering is a property of the session core and
# the core is the only thing TCP and QUIC share.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestL2NoDocumentBytesCrossBeforeBothConfirmations -count=1"
EXPECT="never asked the verifier"
