# docs/red-proofs.md, tier 1: "a signature made inside a ceremony does not name it"
# (P07.S05b, v1.117.180)
#
# The defect: `StampCommitment` is a no-op, so a contribution made inside a proceeding carries no
# `[NibRoster:v:hash]` token. `OneProceeding` is then false on every real ceremony, C19/C01's
# "every signature names its ceremony" is unimplemented, and L3's `ErrProceedingMismatch` arm is
# unreachable because it compares against a field that is always "".
#
# **This is the state P07.S04 actually shipped in, and it held for four slices.** S04 built the
# token format, the reader, D32's version-skew sentence and the check, and no writer — so the
# whole apparatus was reading a field nothing set. It was found at P07.S05b by a tier-4 relay hop
# whose completed signature read `[NibCoSign:1] Accepts p1 [SPKI:]. I accept`.
#
# The guard is behavioural and drives the production door, because the guard that was MEANT to
# catch this was not: `TestTheCommitmentCheckIsLimitedUntilS04` hand-signed its own fixture with an
# explicit empty commitment, so its "S04 has landed" arm could never fire whatever production did.
TIER="tier 1 — go test"
PROVE="go test ./internal/p2p/ -run TestACeremonySignatureNamesItsCeremony -count=1"
EXPECT="a signature that does not name its proceeding"
