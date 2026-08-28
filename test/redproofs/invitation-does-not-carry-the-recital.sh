# docs/red-proofs.md, tier 1: "The reconciliation passes and the recital is empty" (P07.S07b, v1.117.219)
#
# The defect: `NewInvitations` stops copying `r.Intent` into each invitation.
#
# The writer half, and it needs its own row because the COMPARISON is satisfied perfectly by an
# invitation carrying "" against a record carrying "" — `i.Intent != r.Intent` is false when both
# are empty. So every reconciliation test stays green while the signing path has no recital to
# read, and `ErrNoCeremonyIntent` fires at the far end of the ceremony instead. A comparison and
# the value it compares are two facts.
TIER="tier 1 — go test"
PROVE="go test ./internal/ceremony/ -run TestTheInvitationCarriesTheRecordsRecital"
EXPECT="the signing path reads the recital from the invitation"
