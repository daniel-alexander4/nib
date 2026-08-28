# docs/red-proofs.md, tier 1: "Each hop signs its own sentence" (P07.S07b, C15, v1.117.219)
#
# The defect: `StampCommitment` stops overwriting `att.Intent`, so a ceremony signature carries
# whatever the local party typed into their consent screen instead of the record's recital.
#
# D20 makes the recital one sentence with one home. Per-hop intents produce N signatures agreeing
# on a commitment and disagreeing about what they agreed to — and the guard drives the version of
# that which matters, a party signing "I am only witnessing this and agree to nothing" onto a
# document everyone else signed as principals. Every structural check passes: the signature
# verifies, the commitment matches, `OneProceeding` is true.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAPartysTypedIntentIsDiscardedInsideACeremony"
EXPECT="the signed /Reason carries what this party typed"
