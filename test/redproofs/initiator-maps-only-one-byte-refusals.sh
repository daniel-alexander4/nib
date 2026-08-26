# docs/red-proofs.md, tier 1: "the initiator maps only one-byte refusals" (P07.S03a, v1.117.162)
#
# The defect: `Initiate` gates `refusalFor` behind `len(final) == 1` again, which is what it did
# while every refusal was one byte. The named refusal is TWO bytes, so it falls straight past to
# `if !bytes.HasPrefix(final, mySignedPDF)` — and the honest peer that just refused is reported to
# this user as having returned a document that is not the one sent, i.e. accused of a replay.
#
# The discrimination is a property of the BYTES rather than of a length test somebody may relax:
# `refusalFor` checks its own shape, a co-signed document is never two bytes, and every PDF begins
# `%PDF-` (0x25, not 4). Both are driven in `TestARefusalFrameCannotBeMistakenForADocument`.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestAnL3RefusalReachesTheInitiatorByName -count=1"
EXPECT="want ErrNotYourTurn"
