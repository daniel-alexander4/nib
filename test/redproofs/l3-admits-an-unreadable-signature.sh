# docs/red-proofs.md, tier 1: "L3 admits a contribution onto an unreadable signature" (P07.S03, v1.117.159)
#
# The defect: the gate stops asking the document's own verify state and reads the attestations
# alone.
#
# **That is not the same as dropping a redundant check, and the difference is the measurement.** A
# broken signature does not report itself invalid — it VANISHES. A body tamper leaves
# `sign.Verify` reporting `unsigned` with zero signers; a corrupted `/Contents` leaves it
# `invalid`, also with zero. `ReadAttestations` iterates `st.Signers`, so in both cases it returns
# an EMPTY slice, the per-signature `Valid` check cannot fire, and an attestation-only gate reads a
# document carrying an unreadable signature as "nobody has signed yet" — telling the second party
# to wait, while the document can never become valid however many honest blocks are added to it.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAnInvalidPrefixSignatureIsRefused -count=1"
EXPECT="want ErrPrefixUnproven"
