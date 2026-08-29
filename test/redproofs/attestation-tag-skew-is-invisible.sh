# docs/red-proofs.md, tier 1: "The fourth skew surface, and the silent one" (P07.S09c, D32, v1.117.227)
#
# The defect: `Attestations` goes back to requiring `[NibCoSign:1]` verbatim, so a signature from a
# build at `[NibCoSign:2]` matches nothing and arrives with every field empty.
#
# **That is indistinguishable from a signature carrying no Nib attestation at all**, and
# `markOneProceeding` treats an empty commitment on a VALID signature as disqualifying — so ONE
# such signature makes the whole document report *"This document was not produced by a single
# agreed proceeding."* An accusation about the parties, on a document everyone signed correctly,
# caused by one of them updating Nib. It is verbatim the harm the roster token's own version was
# added to prevent one level down; this surface was the one D32 excused.
#
# The check goes red on its SETUP assertion — "this build's own tag version reports 0" — and that
# is the honest reading rather than a weakness: with the version parse gone, no signature reports a
# version at all, so the stimulus the response would be graded against is the thing that vanished.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestANewerAttestationTagIsLegibleAsASkewRatherThanAsSilence"
EXPECT="reports TagVersion 0, want 1"
