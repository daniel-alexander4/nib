# docs/red-proofs.md, tier 1: "the block a party is shown is not the block their key signs"
# (P06.S06, /pending 317, re-opens P07 C09/C15/C19, v1.117.339)
#
# The shipped state for two phases. `StampCommitment` overwrites SIX fields inside a ceremony — the
# roster hash and its version, the recital (the RECORD's, and "whatever the caller put here is
# discarded"), the position, the roster size and the capacity — plus the signer's label. It was
# called at BOTH signing points and at NEITHER quote, while `cosignAttestation`'s own doc claimed it
# "builds the attestation both calls sign over ... so the rendered block and the signed /Reason
# always agree".
#
# So a party read one block at their consent screen and their key signed another. The mutation
# restores exactly that, and the fixture's own SETUP is what makes it visible: it asserts the
# stamped and unstamped blocks DIFFER before comparing them, so a no-op stamp fails as a setup
# failure rather than passing silently.
TIER="tier 1 — go test"
PROVE="go test ./internal/server/ -run TestTheQuotedBlockIsTheBlockThatGetsSigned -count=1"
EXPECT="the roster changed nothing"
